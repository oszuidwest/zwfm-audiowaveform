package render

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"io"
)

// EncodePNG writes img as an 8-bit PNG: RGBA when alpha is true, RGB without
// the alpha channel otherwise, like gdImagePngEx with and without saveAlpha.
// level is the zlib level: -1 for the default, or 0 (none) to 9 (best).
func EncodePNG(w io.Writer, img *image.NRGBA, alpha bool, level int) error {
	if level < -1 || level > 9 {
		return errors.New("Invalid compression level: must be from 0 (none) to 9 (best), or -1 (default)")
	}
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width < 1 || height < 1 {
		return fmt.Errorf("invalid image size %dx%d", width, height)
	}

	bpp, colorType := 3, byte(2)
	if alpha {
		bpp, colorType = 4, 6
	}

	var data bytes.Buffer
	zw, err := zlib.NewWriterLevel(&data, level)
	if err != nil {
		return err
	}
	stride := width * bpp
	prev := make([]byte, stride)
	cur := make([]byte, stride)
	filtered := make([]byte, 1+stride)
	scratch := make([]byte, stride)
	for y := range height {
		row := img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y):]
		if alpha {
			copy(cur, row[:stride])
		} else {
			for x := range width {
				copy(cur[3*x:3*x+3], row[4*x:4*x+3])
			}
		}
		filterRow(filtered, scratch, cur, prev, bpp, level != 0)
		if _, err := zw.Write(filtered); err != nil {
			return err
		}
		prev, cur = cur, prev
	}
	if err := zw.Close(); err != nil {
		return err
	}

	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(height))
	ihdr[8], ihdr[9] = 8, colorType
	// libgd writes a 96 dpi resolution, 3780 pixels per meter.
	var phys [9]byte
	binary.BigEndian.PutUint32(phys[0:], 3780)
	binary.BigEndian.PutUint32(phys[4:], 3780)
	phys[8] = 1

	if _, err := io.WriteString(w, "\x89PNG\r\n\x1a\n"); err != nil {
		return err
	}
	for _, c := range []struct {
		typ  string
		data []byte
	}{{"IHDR", ihdr[:]}, {"pHYs", phys[:]}, {"IDAT", data.Bytes()}, {"IEND", nil}} {
		if err := writeChunk(w, c.typ, c.data); err != nil {
			return err
		}
	}
	return nil
}

func writeChunk(w io.Writer, typ string, data []byte) error {
	var head [8]byte
	binary.BigEndian.PutUint32(head[:4], uint32(len(data)))
	copy(head[4:], typ)
	crc := crc32.NewIEEE()
	crc.Write(head[4:])
	crc.Write(data)
	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], crc.Sum32())
	for _, p := range [][]byte{head[:], data, tail[:]} {
		if _, err := w.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// filterRow writes the filter type and the filtered row cur to dst, using tmp
// as scratch. With adaptive set it picks the filter with the smallest sum of
// absolute values, the heuristic libpng uses; otherwise it stores the row
// unfiltered.
func filterRow(dst, tmp, cur, prev []byte, bpp int, adaptive bool) {
	dst[0] = 0
	copy(dst[1:], cur)
	if !adaptive {
		return
	}
	best := sumAbs(dst[1:])
	for ft := byte(1); ft <= 4; ft++ {
		for i := range cur {
			var a, c byte
			if i >= bpp {
				a, c = cur[i-bpp], prev[i-bpp]
			}
			bb := prev[i]
			var p byte
			switch ft {
			case 1:
				p = a
			case 2:
				p = bb
			case 3:
				p = byte((int(a) + int(bb)) / 2)
			case 4:
				p = paeth(a, bb, c)
			}
			tmp[i] = cur[i] - p
		}
		if s := sumAbs(tmp); s < best {
			best = s
			dst[0] = ft
			copy(dst[1:], tmp)
		}
	}
}

func sumAbs(b []byte) int {
	s := 0
	for _, v := range b {
		s += abs(int(int8(v)))
	}
	return s
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}
