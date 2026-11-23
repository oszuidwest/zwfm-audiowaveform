package sndfile

import (
	"errors"
	"fmt"
	"io"

	"github.com/mewkiz/flac"
)

// flacDecoder converts FLAC frames to 16-bit like libsndfile's
// flac_buffer_copy: wider samples are shifted right, narrower ones left.
type flacDecoder struct {
	stream   *flac.Stream
	channels int
	limit    int64 // frames left per STREAMINFO, -1 if unknown
	pending  []int16
}

func openFLAC(src *source) (*reader, error) {
	stream, err := flac.New(src)
	if err != nil {
		return nil, fmt.Errorf("Error in FLAC file: %w", err)
	}
	info := stream.Info
	if info.NChannels < 1 {
		return nil, errors.New("Error : file has zero channels.")
	}
	d := &flacDecoder{stream: stream, channels: int(info.NChannels), limit: -1}
	frames := int64(info.NSamples)
	if frames > 0 {
		d.limit = frames
	}
	return &reader{
		sampleRate: int(info.SampleRate),
		channels:   int(info.NChannels),
		frames:     frames,
		dec:        d,
	}, nil
}

func (d *flacDecoder) read(dst []int16) (int, error) {
	want := len(dst) / d.channels
	if d.limit >= 0 && int64(want) > d.limit {
		want = int(d.limit)
	}
	got := 0
	for got < want {
		if len(d.pending) == 0 {
			fr, err := d.stream.ParseNext()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					break
				}
				d.limit = 0
				return got, fmt.Errorf("Error in FLAC file: %w", err)
			}
			if len(fr.Subframes) != d.channels {
				return got, errors.New("Error in FLAC file: channel count changed")
			}
			n := int(fr.BlockSize)
			if cap(d.pending) < n*d.channels {
				d.pending = make([]int16, n*d.channels)
			}
			d.pending = d.pending[:n*d.channels]
			shift := 16 - int(fr.BitsPerSample)
			for ch, sub := range fr.Subframes {
				for i, s := range sub.Samples[:n] {
					if shift < 0 {
						d.pending[i*d.channels+ch] = int16(s >> -shift)
					} else {
						d.pending[i*d.channels+ch] = int16(uint16(s) << shift)
					}
				}
			}
		}
		n := min(want-got, len(d.pending)/d.channels)
		copy(dst[got*d.channels:], d.pending[:n*d.channels])
		d.pending = d.pending[n*d.channels:]
		got += n
	}
	if d.limit >= 0 {
		d.limit -= int64(got)
	}
	return got, nil
}
