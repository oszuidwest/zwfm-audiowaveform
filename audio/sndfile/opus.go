package sndfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/thesyncim/gopus"
)

// opusDecoder follows libsndfile's ogg_opus.c: it decodes at the lowest
// Opus rate not below the input rate from the header, applies the header
// gain, drops pre-skip samples, trims the end by the final granule position
// and converts with lrintf(x * 32767) without clipping.
type opusDecoder struct {
	demux    *oggDemux
	dec      *gopus.MultistreamDecoder
	channels int
	srFactor int64
	preskip  int64
	buf      []float32
	out      []float32 // decoded samples of the current packet not yet read
	page     *oggPage
	pktPos   int64
	pgPos    int64
	gpStart  int64
	total    int64 // frames in the stream, -1 until known
	emitted  int64
	eof      bool
}

func openOpus(src *source, demux *oggDemux, first *oggPage) (*reader, error) { //nolint:gocyclo // mirrors libsndfile's Opus header checks
	h := first.packets[0]
	if len(h) < 19 {
		return nil, errors.New("Error in Ogg Opus file: short OpusHead")
	}
	channels := int(h[9])
	preskip := int64(binary.LittleEndian.Uint16(h[10:]))
	inputRate := int(binary.LittleEndian.Uint32(h[12:]))
	gain := int(int16(binary.LittleEndian.Uint16(h[16:]))) //nolint:gosec // output gain is a signed 16-bit field
	family := h[18]
	if channels < 1 {
		return nil, errZeroChannels
	}
	streams, coupled := 1, channels-1
	mapping := []byte{0, 1}[:min(channels, 2)]
	if family != 0 {
		if len(h) < 21+channels {
			return nil, errors.New("Error in Ogg Opus file: short channel mapping")
		}
		streams, coupled = int(h[19]), int(h[20])
		mapping = h[21 : 21+channels]
	} else if channels > 2 {
		return nil, errors.New("Error in Ogg Opus file: bad channel count for mapping family 0")
	}

	var srFactor int64
	switch {
	case inputRate > 24000:
		srFactor = 1
	case inputRate > 16000:
		srFactor = 2
	case inputRate > 12000:
		srFactor = 3
	case inputRate > 8000:
		srFactor = 4
	default:
		srFactor = 6
	}
	rate := 48000 / int(srFactor)
	dec, err := gopus.NewMultistreamDecoder(rate, channels, streams, coupled, mapping)
	if err != nil {
		return nil, fmt.Errorf("Error in Ogg Opus file: %w", err)
	}
	if err := dec.SetGain(gain); err != nil {
		return nil, fmt.Errorf("Error in Ogg Opus file: %w", err)
	}
	d := &opusDecoder{
		demux:    demux,
		dec:      dec,
		channels: channels,
		srFactor: srFactor,
		preskip:  preskip,
		buf:      make([]float32, 5760/int(srFactor)*channels),
		total:    -1,
	}

	// The OpusTags header, then the first audio page.
	page := &oggPage{granule: first.granule, eos: first.eos, packets: first.packets[1:]}
	for len(page.packets) == 0 {
		if page, err = demux.next(); err != nil {
			return nil, errors.New("Error in Ogg Opus file: missing OpusTags")
		}
	}
	page.packets = page.packets[1:]
	if len(page.packets) == 0 {
		page, err = demux.next()
		d.eof = err != nil
	}
	if page != nil {
		// ogg_opus_analyze_file: the granule offset of the stream.
		d.page = page
		d.pgPos = page.granule
		var dur int64
		for _, p := range page.packets {
			if n := opusPacketSamples(p); n > 0 {
				dur += int64(n)
			}
		}
		if !page.eos {
			if dur > d.pgPos {
				return nil, errors.New("Error in Ogg Opus file: first data page's granule position is less than its sample count")
			}
			d.pktPos = d.pgPos - dur
		} else if dur < d.pgPos {
			return nil, errors.New("Error in Ogg Opus file: first data page is also the last and has a granule offset")
		}
	}
	d.gpStart = d.pktPos
	if page != nil && page.eos {
		d.setEnd(page.granule)
	}

	var frames int64
	if end, ok := lastGranule(src, demux.serial); ok {
		d.setEnd(end)
		frames = d.total
	}
	return &reader{sampleRate: rate, channels: channels, frames: frames, dec: d}, nil
}

// refill decodes the next packet into d.out, as ogg_opus_read_refill does.
func (d *opusDecoder) refill() error {
	for d.page == nil || len(d.page.packets) == 0 {
		if d.eof {
			return io.EOF
		}
		page, err := d.demux.next()
		if err != nil {
			d.eof = true
			return err
		}
		d.pktPos = d.pgPos
		d.pgPos = page.granule
		d.page = page
		if page.eos && d.total < 0 {
			d.setEnd(page.granule)
		}
	}
	pkt := d.page.packets[0]
	d.page.packets = d.page.packets[1:]
	if len(d.page.packets) == 0 && d.page.eos {
		d.eof = true
	}
	nsamp, err := d.dec.Decode(pkt, d.buf)
	if err != nil {
		return fmt.Errorf("Error in Ogg Opus file: %w", err)
	}
	n := int64(nsamp)
	pktGranule := d.pktPos + n*d.srFactor
	if pktGranule > d.pgPos {
		if !d.page.eos {
			return errors.New("Error in Ogg Opus file: granule position smaller than the samples on the page")
		}
		n = max((d.pgPos-d.pktPos)/d.srFactor, 0)
	}
	var loc int64
	if d.pktPos < d.preskip {
		loc = min((d.preskip-d.pktPos)/d.srFactor, n)
	}
	d.pktPos = pktGranule
	d.out = d.buf[loc*int64(d.channels) : n*int64(d.channels)]
	return nil
}

// setEnd sets the stream length from the final granule position the way
// ogg_opus_analyze_file computes sf.frames, which sf_readf_short enforces.
func (d *opusDecoder) setEnd(gpEnd int64) {
	d.total = max((gpEnd-d.gpStart-d.preskip)/d.srFactor, 0)
}

func (d *opusDecoder) read(dst []int16) (int, error) {
	want := len(dst) / d.channels
	if d.total >= 0 {
		want = int(min(int64(want), max(d.total-d.emitted, 0)))
	}
	got := 0
	for got < want {
		if len(d.out) == 0 {
			if err := d.refill(); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return got, err
			}
			continue
		}
		n := min(want-got, len(d.out)/d.channels)
		for i, f := range d.out[:n*d.channels] {
			dst[got*d.channels+i] = lrintfShort(f)
		}
		d.out = d.out[n*d.channels:]
		got += n
		if d.total >= 0 {
			want = int(min(int64(want), int64(got)+max(d.total-d.emitted-int64(got), 0)))
		}
	}
	d.emitted += int64(got)
	return got, nil
}

// opusPacketSamples is opus_packet_get_nb_samples at 48 kHz.
func opusPacketSamples(p []byte) int {
	if len(p) == 0 {
		return -1
	}
	toc := p[0]
	var count int
	switch toc & 3 {
	case 0:
		count = 1
	case 1, 2:
		count = 2
	default:
		if len(p) < 2 {
			return -1
		}
		count = int(p[1] & 0x3f)
	}
	config := int(toc >> 3)
	var size int
	switch {
	case config < 12:
		size = []int{480, 960, 1920, 2880}[config&3]
	case config < 16:
		size = []int{480, 960}[config&1]
	default:
		size = []int{120, 240, 480, 960}[config&3]
	}
	n := count * size
	if n*25 > 48000*3 {
		return -1
	}
	return n
}
