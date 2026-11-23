// Package sndfile decodes the audio formats audiowaveform reads through
// libsndfile: WAV (RIFF, RIFX, RF64, BW64), Sony Wave64, raw PCM, FLAC,
// Ogg Vorbis and Ogg Opus. Samples are converted to 16-bit the way
// libsndfile's sf_readf_short does, so the output matches the reference
// implementation.
package sndfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/rmens/audiowaveform-go/audio"
)

// bufferSize is the number of samples per block, as in
// SndFileAudioFileReader::run.
const bufferSize = 16384

// decoder produces interleaved 16-bit samples. read fills dst with whole
// frames and returns how many it wrote. It returns fewer frames than dst can
// hold only at the end of the stream; err is nil at a clean end.
type decoder interface {
	read(dst []int16) (frames int, err error)
}

type reader struct {
	sampleRate int
	channels   int
	frames     int64 // 0 if unknown
	dec        decoder
}

// Run decodes the stream into p with the loop of SndFileAudioFileReader::run:
// blocks of bufferSize/channels frames until a short block, then Done.
func (r *reader) Run(p audio.Processor) error {
	if err := p.Init(r.sampleRate, r.channels, r.frames, bufferSize); err != nil {
		return err
	}
	if !p.ShouldContinue() {
		return nil
	}
	want := bufferSize / r.channels
	buf := make([]int16, want*r.channels)
	for {
		n, rerr := r.dec.read(buf)
		if err := p.Process(buf[:n*r.channels], n); err != nil {
			p.Done()
			return err
		}
		if rerr != nil {
			p.Done()
			return rerr
		}
		if n < want {
			break
		}
	}
	p.Done()
	return nil
}

// Open reads the header and returns a reader. Like libsndfile it detects the
// container from the bytes, not the file name: RIFF/RIFX WAVE, RF64/BW64,
// Sony Wave64, FLAC, Ogg Vorbis and Ogg Opus. A leading ID3v2 tag is skipped.
// If r is also an io.Seeker it is used to learn the length of the stream;
// without one every format still decodes, but the frame count of a stream
// whose header does not state it is reported as 0.
func Open(r io.Reader) (audio.Reader, error) {
	src := newSource(r)
	head, err := src.peek(12)
	if err != nil && len(head) < 4 {
		return nil, errors.New("Format not recognised.")
	}
	// Skip ID3v2 tags, as libsndfile's id3_skip does.
	for len(head) >= 10 && string(head[:3]) == "ID3" {
		h, _ := src.peek(10)
		size := int64(h[6]&0x7f)<<21 | int64(h[7]&0x7f)<<14 | int64(h[8]&0x7f)<<7 | int64(h[9]&0x7f)
		if err := src.skip(size + 10); err != nil {
			return nil, errors.New("Format not recognised.")
		}
		head, _ = src.peek(12)
	}
	switch {
	case len(head) >= 12 && (string(head[:4]) == "RIFF" || string(head[:4]) == "RIFX") && string(head[8:12]) == "WAVE":
		return openWAV(src)
	case len(head) >= 12 && (string(head[:4]) == "RF64" || string(head[:4]) == "BW64") && string(head[8:12]) == "WAVE":
		return openWAV(src)
	case len(head) >= 8 && bytes.Equal(head[:8], []byte{'r', 'i', 'f', 'f', 0x2e, 0x91, 0xcf, 0x11}):
		return openW64(src)
	case len(head) >= 4 && string(head[:4]) == "fLaC":
		return openFLAC(src)
	case len(head) >= 4 && string(head[:4]) == "OggS":
		return openOgg(src)
	}
	return nil, errors.New("Format not recognised.")
}

// OpenRaw returns a reader for headerless PCM. format is one of s8 u8 s16le
// s16be s24le s24be s32le s32be f32le f32be f64le f64be.
func OpenRaw(r io.Reader, channels, sampleRate int, format string) (audio.Reader, error) {
	if channels <= 0 {
		return nil, errors.New("Invalid number of input channels: must be greater than zero")
	}
	if sampleRate <= 0 {
		return nil, errors.New("Invalid input sample rate: must be greater than zero")
	}
	var enc pcmEncoding
	switch format {
	case "s8":
		enc = pcmEncoding{kind: pcmS8, width: 1}
	case "u8":
		enc = pcmEncoding{kind: pcmU8, width: 1}
	case "s16le", "s16be", "s24le", "s24be", "s32le", "s32be":
		enc = pcmEncoding{kind: pcmInt, bigEndian: format[3:] == "be"}
	case "f32le", "f32be":
		enc = pcmEncoding{kind: pcmFloat, width: 4, bigEndian: format[3:] == "be"}
	case "f64le", "f64be":
		enc = pcmEncoding{kind: pcmFloat, width: 8, bigEndian: format[3:] == "be"}
	default:
		return nil, fmt.Errorf("Unsupported format: %s", format)
	}
	if enc.kind == pcmInt {
		switch format[1:3] {
		case "16":
			enc.width = 2
		case "24":
			enc.width = 3
		case "32":
			enc.width = 4
		}
	}
	src := newSource(r)
	block := int64(enc.width * channels)
	var frames int64
	dataBytes := int64(-1)
	if size, ok := src.size(); ok {
		frames = size / block
		dataBytes = frames * block
	}
	return &reader{
		sampleRate: sampleRate,
		channels:   channels,
		frames:     frames,
		dec:        newPCMDecoder(src, enc, channels, dataBytes),
	}, nil
}
