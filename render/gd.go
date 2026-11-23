package render

import (
	"math"
	"slices"
)

// canvas is a truecolor drawing surface with the semantics of the libgd 2.3.3
// primitives that audiowaveform uses. A pixel holds a libgd truecolor value:
// a 7-bit alpha (0 opaque, 127 transparent) in bits 24-30, then red, green
// and blue.
//
// Every write replaces the pixel. libgd blends only when alpha blending is on,
// and audiowaveform turns it off as soon as one color has alpha. With all
// colors opaque, gdAlphaBlend returns the source color, which is a replace too.
type canvas struct {
	w, h int
	pix  []uint32
	ints []int // scratch for filledPolygon, like gdImage.polyInts
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, pix: make([]uint32, w*h)}
}

// gdColor packs a color like gdImageColorAllocateAlpha does for a truecolor
// image; a is the 7-bit libgd alpha.
func gdColor(r, g, b, a uint8) uint32 {
	return uint32(a)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

// setPixel is gdImageSetPixel for a plain color: it ignores points outside
// the clipping rectangle, which is the whole image.
func (c *canvas) setPixel(x, y int, col uint32) {
	if x >= 0 && y >= 0 && x < c.w && y < c.h {
		c.pix[y*c.w+x] = col
	}
}

// clip1d is clip_1d from gd.c: it clips the line (x0,y0)-(x1,y1) to
// min <= x <= max and reports false when nothing is left. The mix of
// truncating casts and implicit double arithmetic follows the C code.
func clip1d(x0, y0, x1, y1, lo, hi int) (int, int, int, int, bool) {
	slope := func() float64 { return float64(y1-y0) / float64(x1-x0) }
	switch {
	case x0 < lo:
		if x1 < lo {
			return x0, y0, x1, y1, false
		}
		m := slope()
		y0 -= int(m * float64(x0-lo))
		x0 = lo
		if x1 > hi {
			y1 = int(float64(y1) + m*float64(hi-x1))
			x1 = hi
		}
	case x0 > hi:
		if x1 > hi {
			return x0, y0, x1, y1, false
		}
		m := slope()
		y0 += int(m * float64(hi-x0))
		x0 = hi
		if x1 < lo {
			y1 -= int(m * float64(x1-lo))
			x1 = lo
		}
	case x1 > hi:
		m := slope()
		y1 += int(m * float64(hi-x1))
		x1 = hi
	case x1 < lo:
		m := slope()
		y1 -= int(m * float64(x1-lo))
		x1 = lo
	}
	return x0, y0, x1, y1, true
}

// line is gdImageLine with a thickness of 1.
func (c *canvas) line(x1, y1, x2, y2 int, col uint32) {
	var ok bool
	if x1, y1, x2, y2, ok = clip1d(x1, y1, x2, y2, 0, c.w-1); !ok {
		return
	}
	if y1, x1, y2, x2, ok = clip1d(y1, x1, y2, x2, 0, c.h-1); !ok {
		return
	}
	// The renderer only draws horizontal and vertical lines.
	if x1 == x2 {
		c.vline(x1, y1, y2, col)
	} else {
		c.hline(y1, x1, x2, col)
	}
}

// hline is gdImageHLine with a thickness of 1.
func (c *canvas) hline(y, x1, x2 int, col uint32) {
	if x2 < x1 {
		x1, x2 = x2, x1
	}
	for ; x1 <= x2; x1++ {
		c.setPixel(x1, y, col)
	}
}

// vline is gdImageVLine with a thickness of 1.
func (c *canvas) vline(x, y1, y2 int, col uint32) {
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	for ; y1 <= y2; y1++ {
		c.setPixel(x, y1, col)
	}
}

// rectangle is gdImageRectangle (an outline) with a thickness of 1.
func (c *canvas) rectangle(x1, y1, x2, y2 int, col uint32) {
	if x1 == x2 && y1 == y2 {
		c.setPixel(x1, y1, col)
		return
	}
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	if x2 < x1 {
		x1, x2 = x2, x1
	}
	if x1 == x2 || y1 == y2 {
		c.line(x1, y1, x2, y2, col)
		return
	}
	c.line(x1, y1, x2, y1, col)
	c.line(x1, y2, x2, y2, col)
	c.line(x1, y1+1, x1, y2-1, col)
	c.line(x2, y1+1, x2, y2-1, col)
}

// filledRectangle is gdImageFilledRectangle: the corners are sorted and then
// clamped to the image.
func (c *canvas) filledRectangle(x1, y1, x2, y2 int, col uint32) {
	if x1 == x2 && y1 == y2 {
		c.setPixel(x1, y1, col)
		return
	}
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	x1, y1 = max(x1, 0), max(y1, 0)
	x2, y2 = min(x2, c.w-1), min(y2, c.h-1)
	for y := y1; y <= y2; y++ {
		row := c.pix[y*c.w:]
		for x := x1; x <= x2; x++ {
			row[x] = col
		}
	}
}

type point struct{ x, y int }

// filledPolygon is gdImageFilledPolygon for a plain color.
func (c *canvas) filledPolygon(p []point, col uint32) {
	n := len(p)
	if n == 0 {
		return
	}
	miny, maxy := p[0].y, p[0].y
	for _, q := range p[1:] {
		miny, maxy = min(miny, q.y), max(maxy, q.y)
	}
	// A flat polygon is drawn as a horizontal line.
	if n > 1 && miny == maxy {
		x1, x2 := p[0].x, p[0].x
		for _, q := range p[1:] {
			if q.x < x1 {
				x1 = q.x
			} else if q.x > x2 {
				x2 = q.x
			}
		}
		c.line(x1, miny, x2, miny, col)
		return
	}
	pmaxy := maxy
	miny, maxy = max(miny, 0), min(maxy, c.h-1)
	for y := miny; y <= maxy; y++ {
		ints := c.ints[:0]
		for i := range n {
			a, b := p[(i+n-1)%n], p[i]
			if a.y == b.y {
				continue
			}
			if a.y > b.y {
				a, b = b, a
			}
			if y >= a.y && y < b.y {
				// libgd divides in float, then adds 0.5 and x1 in double
				// and truncates.
				q := float32((y-a.y)*(b.x-a.x)) / float32(b.y-a.y)
				ints = append(ints, int(float64(q)+0.5+float64(a.x)))
			} else if y == pmaxy && y == b.y {
				ints = append(ints, b.x)
			}
		}
		slices.Sort(ints)
		for i := 0; i+1 < len(ints); i += 2 {
			c.line(ints[i], y, ints[i+1], y, col)
		}
		c.ints = ints
	}
}

// filledArc is gdImageFilledArc with a style that has neither gdChord nor
// gdNoFill set: a filled pie between angles s and e (degrees, clockwise from
// 3 o'clock) of the ellipse w by h around (cx, cy).
func (c *canvas) filledArc(cx, cy, w, h, s, e int, col uint32) {
	if s%360 == e%360 {
		s, e = 0, 360
	} else {
		if s > 360 {
			s %= 360
		}
		if e > 360 {
			e %= 360
		}
		for s < 0 {
			s += 360
		}
		for e < s {
			e += 360
		}
		if s == e {
			s, e = 0, 360
		}
	}

	var pts [363]point
	pti := 1
	var lx, ly, startx, starty, endx, endy int
	for i := s; i <= e; i, pti = i+1, pti+1 {
		x := gdCosT[i%360]*w/(2*1024) + cx
		y := gdSinT[i%360]*h/(2*1024) + cy
		endx, endy = x, y
		if i != s {
			if y == ly {
				// Merge points on the same row: keep the outermost x.
				pti--
				if ((i > 270 || i < 90) && x > lx) || ((i > 90 && i < 270) && x < lx) {
					pts[pti].x = x
				}
			} else {
				pts[pti] = point{x, y}
			}
		} else {
			pts[0] = point{cx, cy}
			startx, starty = x, y
			pts[pti] = point{x, y}
		}
		lx, ly = x, y
	}
	if e-s < 360 {
		// Put back the start and end points if merging dropped them.
		if pts[1].x != startx && pts[1].y == starty {
			copy(pts[2:pti+1], pts[1:pti])
			pts[1] = point{startx, starty}
			pti++
		}
		if pts[pti-1].x != endx && pts[pti-1].y == endy {
			pts[pti] = point{endx, endy}
			pti++
		}
	}
	pts[pti] = point{cx, cy}
	c.filledPolygon(pts[:pti+1], col)
}

// gdCosT and gdSinT reproduce the tables in libgd's gdtables.c: 1024 times
// the cosine and sine of each whole degree, truncated toward zero. The small
// bias makes exact values such as sin(30) = 0.5 come out as 512, as they do
// in the table.
var gdCosT, gdSinT = trigTables()

func trigTables() (cos, sin [360]int) {
	trunc := func(v float64) int {
		return int(math.Copysign(math.Floor(math.Abs(v)+1e-6), v))
	}
	for i := range 360 {
		r := float64(i) * math.Pi / 180
		cos[i] = trunc(1024 * math.Cos(r))
		sin[i] = trunc(1024 * math.Sin(r))
	}
	return cos, sin
}

// fontW and fontH are the glyph size of gdFontSmall.
const fontW, fontH = 6, 13

// smallGlyphs holds the gdFontSmall glyphs for '0'-'9', ':' and '-', taken
// from libgd's gdfonts.c. Each byte is one row; bit 5 is the leftmost pixel.
// The minus shows when a huge start time makes upstream's seconds wrap.
var smallGlyphs = [12][fontH]uint8{
	{0x00, 0x00, 0x00, 0x08, 0x14, 0x22, 0x22, 0x22, 0x22, 0x14, 0x08, 0x00, 0x00}, // '0'
	{0x00, 0x00, 0x00, 0x08, 0x18, 0x28, 0x08, 0x08, 0x08, 0x08, 0x3e, 0x00, 0x00}, // '1'
	{0x00, 0x00, 0x00, 0x1c, 0x22, 0x02, 0x04, 0x08, 0x10, 0x20, 0x3e, 0x00, 0x00}, // '2'
	{0x00, 0x00, 0x00, 0x1c, 0x22, 0x02, 0x0c, 0x02, 0x02, 0x22, 0x1c, 0x00, 0x00}, // '3'
	{0x00, 0x00, 0x00, 0x04, 0x0c, 0x14, 0x24, 0x24, 0x3e, 0x04, 0x04, 0x00, 0x00}, // '4'
	{0x00, 0x00, 0x00, 0x3e, 0x20, 0x3c, 0x22, 0x02, 0x02, 0x22, 0x1c, 0x00, 0x00}, // '5'
	{0x00, 0x00, 0x00, 0x0e, 0x10, 0x20, 0x3c, 0x22, 0x22, 0x22, 0x1c, 0x00, 0x00}, // '6'
	{0x00, 0x00, 0x00, 0x3e, 0x02, 0x04, 0x04, 0x08, 0x08, 0x10, 0x10, 0x00, 0x00}, // '7'
	{0x00, 0x00, 0x00, 0x1c, 0x22, 0x22, 0x1c, 0x22, 0x22, 0x22, 0x1c, 0x00, 0x00}, // '8'
	{0x00, 0x00, 0x00, 0x1c, 0x22, 0x22, 0x22, 0x1e, 0x02, 0x04, 0x38, 0x00, 0x00}, // '9'
	{0x00, 0x00, 0x00, 0x00, 0x00, 0x0c, 0x0c, 0x00, 0x00, 0x0c, 0x0c, 0x00, 0x00}, // ':'
	{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3e, 0x00, 0x00, 0x00, 0x00, 0x00}, // '-'
}

// char is gdImageChar with gdFontSmall. Only digits, ':' and '-' are drawn;
// the renderer writes nothing else.
func (c *canvas) char(x, y int, ch byte, col uint32) {
	var g *[fontH]uint8
	switch {
	case ch >= '0' && ch <= '9':
		g = &smallGlyphs[ch-'0']
	case ch == ':':
		g = &smallGlyphs[10]
	case ch == '-':
		g = &smallGlyphs[11]
	default:
		return
	}
	for row, bits := range g {
		for px := range fontW {
			if bits&(1<<(fontW-1-px)) != 0 {
				c.setPixel(x+px, y+row, col)
			}
		}
	}
}

// str is gdImageString with gdFontSmall.
func (c *canvas) str(x, y int, s string, col uint32) {
	for i := range len(s) {
		c.char(x, y, s[i], col)
		x += fontW
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
