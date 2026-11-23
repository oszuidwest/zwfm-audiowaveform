// Package render draws waveform data as an image, pixel for pixel like the
// GdImageRenderer of audiowaveform built with libgd 2.3.3.
package render

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/rmens/audiowaveform-go/waveform"
)

// Colors is a color scheme. Waveform holds one color per channel; channels
// beyond its length cycle through it.
type Colors struct {
	Border, Background, AxisLabel color.NRGBA
	Waveform                      []color.NRGBA
}

// Audacity and Audition are the two built-in color schemes of audiowaveform.
var (
	Audacity = Colors{
		Border:     color.NRGBA{0, 0, 0, 255},
		Background: color.NRGBA{214, 214, 214, 255},
		Waveform:   []color.NRGBA{{63, 77, 155, 255}},
		AxisLabel:  color.NRGBA{0, 0, 0, 255},
	}
	Audition = Colors{
		Border:     color.NRGBA{157, 157, 157, 255},
		Background: color.NRGBA{0, 63, 34, 255},
		Waveform:   []color.NRGBA{{134, 252, 199, 255}},
		AxisLabel:  color.NRGBA{190, 190, 190, 255},
	}
)

// HasAlpha reports whether any color is not fully opaque. Such an image is
// drawn without blending and its PNG keeps the alpha channel.
func (c Colors) HasAlpha() bool {
	translucent := func(v color.NRGBA) bool { return v.A != 255 }
	return translucent(c.Border) || translucent(c.Background) || translucent(c.AxisLabel) ||
		slices.ContainsFunc(c.Waveform, translucent)
}

// Options controls how Render draws.
type Options struct {
	Width, Height int
	StartTime     float64 // seconds
	AxisLabels    bool
	Bars          bool
	BarWidth      int
	BarGap        int
	RoundedBars   bool
	// AutoAmplitudeScale scales the loudest visible point to full height and
	// overrides AmplitudeScale.
	AutoAmplitudeScale bool
	AmplitudeScale     float64
	Colors             Colors
}

// Render draws d. The returned pixels carry the alpha that a PNG written by
// audiowaveform holds: libgd keeps 7 bits of alpha, so a color with alpha a
// ends up as 255 - 2*q - q/64 with q = 127 - a/2.
func Render(d *waveform.Data, o Options) (*image.NRGBA, error) {
	if o.StartTime < 0 {
		return nil, errors.New("Invalid start time: minimum 0")
	}
	if o.Bars {
		if o.BarWidth < 1 {
			return nil, errors.New("Invalid bar width: minimum 1")
		}
		if o.BarGap < 0 {
			return nil, errors.New("Invalid bar gap: minimum 0")
		}
	}
	if o.Width < 1 {
		return nil, errors.New("Invalid image width: minimum 1")
	}
	if o.Height < 1 {
		return nil, errors.New("Invalid image height: minimum 1")
	}
	if d.SampleRate <= 0 {
		return nil, fmt.Errorf("Invalid sample rate: %d Hz", d.SampleRate)
	}
	if d.Channels < 1 {
		return nil, fmt.Errorf("Invalid number of channels: %d", d.Channels)
	}
	if d.Len() < 1 {
		return nil, errors.New("Empty waveform buffer")
	}
	if d.SamplesPerPixel < 1 {
		return nil, fmt.Errorf("Invalid waveform scale: %d", d.SamplesPerPixel)
	}
	if len(o.Colors.Waveform) == 0 {
		return nil, errors.New("Invalid waveform colors: need at least one")
	}
	// gdImageCreateTrueColor refuses images whose pixel count overflows an int.
	if int64(o.Width)*int64(o.Height) > math.MaxInt32 {
		return nil, errors.New("Failed to create image")
	}

	r := &renderer{
		d:          d,
		o:          o,
		c:          newCanvas(o.Width, o.Height),
		sampleRate: d.SampleRate,
		spp:        d.SamplesPerPixel,
	}
	r.startIndex = r.secondsToPixels(o.StartTime)
	r.draw()
	return r.c.nrgba(), nil
}

type renderer struct {
	d          *waveform.Data
	o          Options
	c          *canvas
	sampleRate int
	spp        int
	startIndex int

	border, background, axisLabel uint32
	waveform                      []uint32
}

// gdColorOf converts a color like GdImageRenderer::createColor: a color with
// alpha gets the libgd alpha 127 - a/2.
func gdColorOf(v color.NRGBA) uint32 {
	var a uint8
	if v.A != 255 {
		a = 127 - v.A/2
	}
	return gdColor(v.R, v.G, v.B, a)
}

func (r *renderer) draw() {
	cs := r.o.Colors
	r.border = gdColorOf(cs.Border)
	r.background = gdColorOf(cs.Background)
	for _, v := range cs.Waveform {
		r.waveform = append(r.waveform, gdColorOf(v))
	}
	r.axisLabel = gdColorOf(cs.AxisLabel)

	r.c.filledRectangle(0, 0, r.o.Width-1, r.o.Height-1, r.background)
	if r.o.Bars {
		r.drawBars()
	} else {
		r.drawWaveform()
	}
	if r.o.AxisLabels {
		r.c.rectangle(0, 0, r.o.Width-1, r.o.Height-1, r.border)
		r.drawTimeAxisLabels()
	}
}

func (r *renderer) secondsToPixels(seconds float64) int {
	return int(seconds * float64(r.sampleRate) / float64(r.spp))
}

func (r *renderer) amplitudeScale() float64 {
	if !r.o.AutoAmplitudeScale {
		return r.o.AmplitudeScale
	}
	end := min(r.startIndex+r.o.Width, r.d.Len())
	if end <= r.startIndex {
		// The start is at or past the end of the data. Upstream scans an
		// empty range here and divides by the unset limits INT_MIN and
		// INT_MAX; past the end it reads out of bounds and crashes.
		return 32767.0 / -math.MinInt32
	}
	return waveform.AmplitudeScale(r.d, r.startIndex, end)
}

// rows calls fn for each channel with the top row and the height of the
// channel's strip, splitting the drawable height like upstream does.
func (r *renderer) rows(fn func(channel, top, height int)) {
	topY, bottomY := 0, r.o.Height-1
	if r.o.AxisLabels {
		// Keep off the top and bottom border.
		topY, bottomY = 1, r.o.Height-2
	}
	channels := r.d.Channels
	available := bottomY - topY + 1
	rowHeight := available / channels
	top := topY
	for ch := range channels {
		bottom := top + rowHeight
		if ch == channels-1 {
			bottom = top + available - 1
		}
		fn(ch, top, bottom-top+1)
		available -= rowHeight + 1
		top += rowHeight + 1
	}
}

// toY maps a sample in [-32768, 32767] to a row of a strip. The product is
// an int in C, so it wraps the same way for strips over 32768 rows.
func toY(sample int16, top, height int) int {
	v := int32(sample) + 32768
	return top + height - 1 - int(v*int32(height)/65536)
}

func (r *renderer) waveformColor(channel int) uint32 {
	return r.waveform[channel%len(r.waveform)]
}

func (r *renderer) drawWaveform() {
	scale := r.amplitudeScale()
	size := r.d.Len()
	r.rows(func(ch, top, height int) {
		col := r.waveformColor(ch)
		for i, x := r.startIndex, 0; x < r.o.Width && i < size; i, x = i+1, x+1 {
			low := waveform.ScaleSample(int(r.d.Min(ch, i)), scale)
			high := waveform.ScaleSample(int(r.d.Max(ch, i)), scale)
			r.c.line(x, toY(high, top, height), x, toY(low, top, height), col)
		}
	})
}

// barHeight returns the largest magnitude, limited to 32767, of the points
// start to start+width-1 of a channel.
func (r *renderer) barHeight(ch, start, width int) int {
	size := r.d.Len()
	if start >= size {
		return 0
	}
	low, high := math.MaxInt, math.MinInt
	for i, idx := 0, start; i < width && idx < size; i, idx = i+1, idx+1 {
		low = min(low, int(r.d.Min(ch, idx)))
		high = max(high, int(r.d.Max(ch, idx)))
	}
	low = min(abs(low), math.MaxInt16)
	high = min(abs(high), math.MaxInt16)
	return max(low, high)
}

func (r *renderer) drawBars() {
	scale := r.amplitudeScale()
	barWidth := r.o.BarWidth
	total := barWidth + r.o.BarGap
	// Start at the bar that holds the start index.
	barStart := r.startIndex / total * total
	offset := barStart - r.startIndex
	r.rows(func(ch, top, height int) {
		col := r.waveformColor(ch)
		for i, x := barStart, offset; x < r.o.Width; i, x = i+total, x+total {
			h := r.barHeight(ch, i, total)
			y1 := toY(waveform.ScaleSample(h, scale), top, height)
			y2 := toY(waveform.ScaleSample(-h, scale), top, height)
			if y1 == y2 {
				continue
			}
			if r.o.RoundedBars && barWidth > 2 && height >= 3 {
				radius := barWidth / 2
				if barWidth > 4 {
					radius = barWidth / 4
				}
				r.roundedRectangle(x, y1, x+barWidth-1, y2, radius, col)
			} else {
				r.c.filledRectangle(x, y1, x+barWidth-1, y2, col)
			}
		}
	})
}

func (r *renderer) roundedRectangle(left, top, right, bottom, radius int, col uint32) {
	lx, ty := left+radius, top+radius
	rx, by := right-radius, bottom-radius
	d := radius * 2
	r.c.filledRectangle(left, ty, right, by, col)
	r.c.filledArc(lx, ty, d, d, 180, 270, col)
	r.c.filledArc(rx, ty, d, d, 270, 0, col)
	r.c.filledRectangle(lx, top, rx, ty, col)
	r.c.filledArc(lx, by, d, d, 90, 180, col)
	r.c.filledArc(rx, by, d, d, 0, 90, col)
	r.c.filledRectangle(lx, by, rx, bottom, col)
}

func (r *renderer) drawTimeAxisLabels() {
	const markerHeight = 10
	interval := r.axisLabelScale()
	first := roundUpToNearest(r.o.StartTime, interval)
	offsetSecs := float64(first) - r.o.StartTime
	offsetPixels := int(float64(r.sampleRate)*offsetSecs) / r.spp
	w, h := r.o.Width, r.o.Height
	for secs := first; ; secs += interval {
		x := int(int64(offsetPixels) + int64(secs-first)*int64(r.sampleRate)/int64(r.spp))
		if x >= w {
			break
		}
		r.c.line(x, 0, x, markerHeight, r.border)
		r.c.line(x, h-1, x, h-1-markerHeight, r.border)

		label := timeLabel(secs)
		labelX := x - fontW*len(label)/2 + 1
		labelY := h - 1 - markerHeight - 1 - fontH
		if labelX >= 0 {
			r.c.str(labelX, labelY, label, r.axisLabel)
		}
	}
}

// axisLabelScale returns the seconds between axis markers: the first of 1, 2,
// 5, 10, 20, 30 seconds, then minutes, then hours, that puts the markers at
// least 60 pixels apart.
func (r *renderer) axisLabelScale() int {
	steps := [...]int{1, 2, 5, 10, 20, 30}
	base, i := 1, 0
	for {
		secs := base * steps[i]
		if r.secondsToPixels(float64(secs)) >= 60 {
			return secs
		}
		if i++; i == len(steps) {
			base *= 60
			i = 0
		}
	}
}

// roundUpToNearest is MathUtil::roundUpToNearest.
func roundUpToNearest(value float64, multiple int) int {
	if multiple == 0 {
		return 0
	}
	sign := 1
	if value < 0 {
		sign, value = -1, -value
	}
	up := int(math.Ceil(value))
	return sign * ((up + multiple - 1) / multiple) * multiple
}

// timeLabel is TimeUtil::secondsToString: mm:ss, or hh:mm:ss from one hour.
func timeLabel(secs int) string {
	h := secs / 3600
	secs -= h * 3600
	m := secs / 60
	secs -= m * 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, secs)
	}
	return fmt.Sprintf("%02d:%02d", m, secs)
}

// nrgba converts the canvas like gd_png.c does when it writes alpha: the
// 7-bit libgd alpha q becomes 255 - ((q << 1) + (q >> 6)).
func (c *canvas) nrgba() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.w, c.h))
	for i, p := range c.pix {
		q := p >> 24 & 0x7f
		img.Pix[4*i] = uint8(p >> 16)
		img.Pix[4*i+1] = uint8(p >> 8)
		img.Pix[4*i+2] = uint8(p)
		img.Pix[4*i+3] = uint8(255 - (q<<1 + q>>6))
	}
	return img
}
