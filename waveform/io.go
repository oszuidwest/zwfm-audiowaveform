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

var errInvalidBits = errors.New("Invalid bits: must be either 8 or 16")

// ErrTruncated means the data ended before the length in its header.
var ErrTruncated = errors.New("Unexpected end of file")

// ReadDat reads the binary waveform data format (version 1 or 2).
//
// If the input ends early, ReadDat returns the fields and values read so far
// together with ErrTruncated. audiowaveform accepts such a file but not such
// input on stdin.
func ReadDat(r io.Reader) (*Data, error) {
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

	size := uint64(uint32(header[4])) * uint64(d.Channels) * 2 // values to read
	width := uint64(2)
	if header[1]&flag8Bit != 0 {
		d.Bits, width = 8, 1
	}
	raw, err := io.ReadAll(io.LimitReader(br, int64(min(size*width, 1<<62))))
	if err != nil {
		return nil, err
	}
	var err2 error
	if uint64(len(raw)) < size*width {
		err2 = ErrTruncated
	}
	d.Samples = make([]int16, len(raw)/int(width))
	for i := range d.Samples {
		if width == 1 {
			d.Samples[i] = int16(int8(raw[i])) * 256
		} else {
			d.Samples[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
		}
	}
	return d, err2
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// WriteDat writes the binary format: version 1 for one channel, else 2.
func (d *Data) WriteDat(w io.Writer, bits int) error {
	if bits != 8 && bits != 16 {
		return errInvalidBits
	}
	version, flags := int32(1), uint32(0)
	if d.Channels != 1 {
		version = 2
	}
	if bits == 8 {
		flags = flag8Bit
	}
	bw := bufio.NewWriter(w)
	header := []any{version, flags, int32(d.SampleRate), int32(d.SamplesPerPixel), uint32(d.Len())}
	if version == 2 {
		header = append(header, int32(d.Channels))
	}
	for _, v := range header {
		if err := binary.Write(bw, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	if bits == 8 {
		for i := range d.Len() {
			for c := range d.Channels {
				bw.WriteByte(byte(int8(d.Min(c, i) / 256)))
				bw.WriteByte(byte(int8(d.Max(c, i) / 256)))
			}
		}
	} else if err := binary.Write(bw, binary.LittleEndian, d.Samples); err != nil {
		return err
	}
	return bw.Flush()
}

// WriteJSON writes the JSON format (always version 2).
func (d *Data) WriteJSON(w io.Writer, bits int) error {
	if bits != 8 && bits != 16 {
		return errInvalidBits
	}
	div := int16(1)
	if bits == 8 {
		div = 256
	}
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, `{"version":2,"channels":%d,"sample_rate":%d,"samples_per_pixel":%d,"bits":%d,"length":%d,"data":[`,
		d.Channels, d.SampleRate, d.SamplesPerPixel, bits, d.Len())
	var buf []byte
	for i, v := range d.Samples {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendInt(buf, int64(v/div), 10)
		if len(buf) > 4096 {
			bw.Write(buf)
			buf = buf[:0]
		}
	}
	bw.Write(buf)
	bw.WriteString("]}\n")
	return bw.Flush()
}

// WriteText writes one line per point with comma-separated min,max pairs for
// each channel.
func (d *Data) WriteText(w io.Writer, bits int) error {
	if bits != 8 && bits != 16 {
		return errInvalidBits
	}
	div := int16(1)
	if bits == 8 {
		div = 256
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
		bw.Write(buf)
	}
	return bw.Flush()
}
