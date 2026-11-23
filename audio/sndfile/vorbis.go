package sndfile

import (
	"errors"
	"io"

	"github.com/jfreymuth/oggvorbis/vorbis"
)

// vorbisDecoder follows libsndfile's ogg_vorbis.c: samples are converted
// with lrintf(x * 32767) without clipping, the stream starts at the granule
// offset of the first audio page and ends at the last page's granule
// position.
type vorbisDecoder struct {
	demux    *oggDemux
	dec      vorbis.Decoder
	channels int
	buf      []float32
	pcm      []float32 // backing array of pending
	pending  []float32 // decoded, interleaved
	decoded  int64     // frames decoded so far
	pcmStart int64
	limit    int64 // frames left, -1 if unknown
	eof      bool
}

func openVorbis(src *source, demux *oggDemux, first *oggPage) (*reader, error) {
	d := &vorbisDecoder{demux: demux, limit: -1}
	page := first
	for range 3 {
		for len(page.packets) == 0 {
			var err error
			if page, err = demux.next(); err != nil {
				return nil, errors.New("Error in Ogg Vorbis file: missing headers")
			}
		}
		if err := d.dec.ReadHeader(page.packets[0]); err != nil {
			return nil, errors.New("Error in Ogg Vorbis file: " + err.Error())
		}
		page.packets = page.packets[1:]
	}
	d.channels = d.dec.Channels()
	if d.channels < 1 {
		return nil, errZeroChannels
	}
	d.buf = make([]float32, d.dec.BufferSize())

	// Decode the first audio page to find the granule offset of the stream,
	// as vorbis_calculate_granulepos does.
	if len(page.packets) == 0 {
		var err error
		page, err = demux.next()
		d.eof = err != nil
	}
	if page != nil {
		d.decodePage(page)
		d.pcmStart = page.granule - d.decoded
		if d.pcmStart < 0 || (page.eos && page.granule <= d.decoded) || page.granule == -1 {
			d.pcmStart = 0
		}
		d.trim(page)
	}

	var frames int64
	if end, ok := lastGranule(src, demux.serial); ok && end > d.pcmStart {
		frames = end - d.pcmStart
		d.limit = frames
	}
	return &reader{
		sampleRate: d.dec.SampleRate(),
		channels:   d.channels,
		frames:     frames,
		dec:        d,
	}, nil
}

// decodePage decodes the packets of page into pending, which must be empty.
func (d *vorbisDecoder) decodePage(page *oggPage) {
	d.pending = d.pcm[:0]
	for _, pkt := range page.packets {
		out, err := d.dec.DecodeInto(pkt, d.buf)
		if err != nil {
			continue // libsndfile skips packets that fail to decode
		}
		d.pending = append(d.pending, out...)
		d.decoded += int64(len(out) / d.channels)
	}
	d.pcm = d.pending
}

// trim drops samples decoded past the page's granule position (end padding).
func (d *vorbisDecoder) trim(page *oggPage) {
	if page.granule == -1 {
		return
	}
	if extra := d.decoded - (page.granule - d.pcmStart); extra > 0 {
		extra = min(extra, int64(len(d.pending)/d.channels))
		d.pending = d.pending[:len(d.pending)-int(extra)*d.channels]
		d.decoded -= extra
	}
}

func (d *vorbisDecoder) read(dst []int16) (int, error) {
	want := len(dst) / d.channels
	if d.limit >= 0 && int64(want) > d.limit {
		want = int(d.limit)
	}
	got := 0
	for got < want {
		if len(d.pending) == 0 {
			if d.eof {
				break
			}
			page, err := d.demux.next()
			if err != nil {
				d.eof = true
				if errors.Is(err, io.EOF) {
					break
				}
				return got, err
			}
			d.decodePage(page)
			d.trim(page)
			if page.eos {
				d.eof = true
			}
			continue
		}
		n := min(want-got, len(d.pending)/d.channels)
		for i, f := range d.pending[:n*d.channels] {
			dst[got*d.channels+i] = lrintfShort(f)
		}
		d.pending = d.pending[n*d.channels:]
		got += n
	}
	if d.limit >= 0 {
		d.limit -= int64(got)
	}
	return got, nil
}
