package waveform

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
)

const flag8Bit = 0x00000001

// chunk is the size of the buffers that values are encoded and decoded in.
// It is even, so a chunk holds whole 16-bit values.
const chunk = 4096

var errInvalidBits = errors.New("Invalid bits: must be either 8 or 16")

// divisor returns what a 16-bit value is divided by to write it with bits.
func divisor(bits int) (int16, error) {
	switch bits {
	case 8:
		return 256, nil
	case 16:
		return 1, nil
	}
	return 0, errInvalidBits
}

// ErrTruncated means the data ended before the length in its header.
var ErrTruncated = errors.New("Unexpected end of file")

// ReadDat reads the binary waveform data format (version 1 or 2).
//
// If the input ends early, ReadDat returns the fields and values read so far
// together with ErrTruncated. audiowaveform accepts such a file but not such
// input on stdin.
func ReadDat(r io.Reader) (*Data, error) { //nolint:gocyclo // mirrors WaveformBuffer::load and its error order
	d := New()
	br := bufio.NewReader(r)
	var header [6]int32
	n := 0
	for ; n < len(header); n++ {
		if n == 5 && header[0] != 2 {
			break // version 1 has no channel count
		}
		if err := binary.Read(br, binary.LittleEndian, &header[n]); err != nil {
			if isEOF(err) {
				return d, ErrTruncated
			}
			return nil, err
		}
		switch n {
		case 0:
			if v := header[0]; v != 1 && v != 2 {
				return nil, fmt.Errorf("Cannot load data file version: %d", v)
			}
		case 2:
			if d.SampleRate = int(header[2]); d.SampleRate < 1 {
				return nil, fmt.Errorf("Invalid sample rate: %d Hz, minimum 1 Hz", d.SampleRate)
			}
		case 3:
			if d.SamplesPerPixel = int(header[3]); d.SamplesPerPixel < 2 {
				return nil, fmt.Errorf("Invalid samples per pixel: %d, minimum 2", d.SamplesPerPixel)
			}
		case 5:
			d.Channels = int(header[5])
		}
	}
	if d.Channels < 1 || d.Channels > MaxChannels {
		return nil, fmt.Errorf("Cannot load data file with %d channels", d.Channels)
	}

	size := uint64(uint32(header[4])) * uint64(d.Channels) * 2 //nolint:gosec // values to read; the length field is unsigned 32-bit
	width := 2
	if header[1]&flag8Bit != 0 {
		d.Bits, width = 8, 1
	}
	lr := io.LimitReader(br, int64(min(size*uint64(width), 1<<62)))
	var buf [chunk]byte
	for {
		n, err := io.ReadFull(lr, buf[:])
		// A last odd byte of 16-bit data is dropped.
		for i := 0; i+width <= n; i += width {
			if width == 1 {
				d.Samples = append(d.Samples, int16(int8(buf[i]))*256) //nolint:gosec // signed 8-bit .dat value
			} else {
				d.Samples = append(d.Samples, int16(binary.LittleEndian.Uint16(buf[i:]))) //nolint:gosec // signed 16-bit .dat value
			}
		}
		if isEOF(err) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if uint64(len(d.Samples)) < size {
		return d, ErrTruncated
	}
	return d, nil
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// WriteDat writes the binary format: version 1 for one channel, else 2.
func (d *Data) WriteDat(w io.Writer, bits int) error {
	div, err := divisor(bits)
	if err != nil {
		return err
	}
	version, flags := int32(1), uint32(0)
	if d.Channels != 1 {
		version = 2
	}
	samples := d.Samples
	if bits == 8 {
		flags = flag8Bit
		// Like upstream, only 16-bit output keeps the values of a last,
		// partial point.
		samples = samples[:2*d.Channels*d.Len()]
	}
	bw := bufio.NewWriter(w)
	header := []any{version, flags, int32(d.SampleRate), int32(d.SamplesPerPixel), uint32(d.Len())} //nolint:gosec // .dat header field width
	if version == 2 {
		header = append(header, int32(d.Channels)) //nolint:gosec // .dat header field width
	}
	for _, v := range header {
		if err := binary.Write(bw, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	buf := make([]byte, 0, chunk)
	for _, v := range samples {
		if v /= div; bits == 8 {
			buf = append(buf, byte(int8(v))) //nolint:gosec // 8-bit .dat stores the low byte
		} else {
			buf = binary.LittleEndian.AppendUint16(buf, uint16(v)) //nolint:gosec // two's complement bits of a 16-bit value
		}
		if len(buf) == chunk {
			if _, err := bw.Write(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
	if _, err := bw.Write(buf); err != nil {
		return err
	}
	return bw.Flush()
}

// WriteJSON writes the JSON format (always version 2).
func (d *Data) WriteJSON(w io.Writer, bits int) error {
	div, err := divisor(bits)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	buf := fmt.Appendf(nil, `{"version":2,"channels":%d,"sample_rate":%d,"samples_per_pixel":%d,"bits":%d,"length":%d,"data":[`,
		d.Channels, d.SampleRate, d.SamplesPerPixel, bits, d.Len())
	for i, v := range d.Samples {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendInt(buf, int64(v/div), 10)
		if len(buf) > chunk {
			if _, err := bw.Write(buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
	buf = append(buf, "]}\n"...)
	if _, err := bw.Write(buf); err != nil {
		return err
	}
	return bw.Flush()
}

// WriteText writes one line per point with comma-separated min,max pairs for
// each channel.
func (d *Data) WriteText(w io.Writer, bits int) error {
	div, err := divisor(bits)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	var buf []byte
	for i := range d.Len() {
		buf = buf[:0]
		for c := range d.Channels {
			if c > 0 {
				buf = append(buf, ',')
			}
			buf = strconv.AppendInt(buf, int64(d.Min(c, i)/div), 10)
			buf = append(buf, ',')
			buf = strconv.AppendInt(buf, int64(d.Max(c, i)/div), 10)
		}
		buf = append(buf, '\n')
		if _, err := bw.Write(buf); err != nil {
			return err
		}
	}
	return bw.Flush()
}
