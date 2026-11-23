package sndfile

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

type pcmKind int

const (
	pcmInt   pcmKind = iota // signed integer, 2 to 4 bytes
	pcmU8                   // unsigned 8-bit
	pcmS8                   // signed 8-bit
	pcmFloat                // IEEE float, 4 or 8 bytes
)

type pcmEncoding struct {
	kind      pcmKind
	width     int // bytes per sample
	bigEndian bool
}

// pcmDecoder reads uncompressed samples and converts them like libsndfile's
// pcm.c, float32.c and double64.c.
type pcmDecoder struct {
	r         io.Reader
	enc       pcmEncoding
	channels  int
	remaining int64 // bytes left in the data chunk, -1 if unknown
	raw       []byte
}

// newPCMDecoder reads at most dataBytes bytes (whole frames), or up to the
// end of r when dataBytes is negative.
func newPCMDecoder(r io.Reader, enc pcmEncoding, channels int, dataBytes int64) *pcmDecoder {
	return &pcmDecoder{r: r, enc: enc, channels: channels, remaining: dataBytes}
}

func (d *pcmDecoder) read(dst []int16) (int, error) {
	block := d.enc.width * d.channels
	need := len(dst) / d.channels * block
	if d.remaining >= 0 && int64(need) > d.remaining {
		need = int(d.remaining / int64(block) * int64(block))
	}
	if cap(d.raw) < need {
		d.raw = make([]byte, need)
	}
	raw := d.raw[:need]
	n, err := io.ReadFull(d.r, raw)
	if d.remaining >= 0 {
		d.remaining -= int64(n)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = nil
	}
	frames := n / block
	d.convert(raw[:frames*block], dst[:frames*d.channels])
	return frames, err
}

func (d *pcmDecoder) convert(b []byte, dst []int16) {
	var order binary.ByteOrder = binary.LittleEndian
	if d.enc.bigEndian {
		order = binary.BigEndian
	}
	w := d.enc.width
	switch {
	case d.enc.kind == pcmU8:
		for i := range dst {
			dst[i] = int16((int32(b[i]) - 0x80) << 8)
		}
	case d.enc.kind == pcmS8:
		for i := range dst {
			dst[i] = int16(int8(b[i])) << 8
		}
	case d.enc.kind == pcmInt && w == 2:
		for i := range dst {
			dst[i] = int16(order.Uint16(b[2*i:]))
		}
	case d.enc.kind == pcmInt && w == 3:
		// The top two bytes of each 24-bit sample.
		for i := range dst {
			s := b[3*i : 3*i+3]
			if d.enc.bigEndian {
				dst[i] = int16(uint16(s[0])<<8 | uint16(s[1]))
			} else {
				dst[i] = int16(uint16(s[2])<<8 | uint16(s[1]))
			}
		}
	case d.enc.kind == pcmInt && w == 4:
		for i := range dst {
			dst[i] = int16(int32(order.Uint32(b[4*i:])) >> 16)
		}
	case d.enc.kind == pcmFloat && w == 4:
		for i := range dst {
			dst[i] = floatToShort(math.Float32frombits(order.Uint32(b[4*i:])))
		}
	case d.enc.kind == pcmFloat && w == 8:
		for i := range dst {
			dst[i] = floatToShort(float32(math.Float64frombits(order.Uint64(b[8*i:]))))
		}
	}
}

// floatToShort is audiowaveform's static_cast<short>(f * 32767) as built for
// arm64: the float product is truncated to a saturated 32-bit integer
// (fcvtzs) and then narrowed to 16 bits, so values past full scale wrap.
func floatToShort(f float32) int16 {
	x := f * 32767
	switch {
	case x != x: // NaN
		return 0
	case x >= 2147483647:
		return -1 // low 16 bits of 0x7fffffff
	case x <= -2147483648:
		return 0 // low 16 bits of -0x80000000
	}
	return int16(int32(x))
}

// lrintfShort is libsndfile's psf_lrintf(f * 32767.0f) stored into a short,
// used by the Vorbis and Opus decoders: round half to even, no clipping.
func lrintfShort(f float32) int16 {
	x := float64(f * 32767)
	return int16(int64(math.RoundToEven(x)))
}
