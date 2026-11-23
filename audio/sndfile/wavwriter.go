package sndfile

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"

	"github.com/oszuidwest/zwfm-audiowaveform/audio"
)

// WAVWriter writes 16-bit PCM WAV, like audiowaveform's WavFileWriter. The
// 44-byte header matches what libsndfile writes for SF_FORMAT_WAV |
// SF_FORMAT_PCM_16. If the destination is an io.WriteSeeker the sizes are
// filled in when Done is called. Otherwise (a pipe, which libsndfile refuses)
// the header carries the frame count given to Init, or 0xffffffff sizes when
// that is unknown.
type WAVWriter struct {
	w        io.Writer
	bw       *bufio.Writer
	channels int
	frames   int64
	seeker   io.WriteSeeker // nil when the destination cannot seek
	start    int64          // offset of the header in seeker
	buf      []byte
	err      error
}

// NewWAVWriter returns a processor that writes the audio it receives to w.
func NewWAVWriter(w io.Writer) *WAVWriter {
	return &WAVWriter{w: w}
}

func wavHeader(sampleRate, channels int, dataBytes int64) []byte {
	h := make([]byte, 44)
	riff, data := uint32(0xffffffff), uint32(0xffffffff)
	if dataBytes >= 0 && dataBytes+36 <= 0xffffffff {
		riff, data = uint32(dataBytes+36), uint32(dataBytes)
	}
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], riff)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)                             // WAVE_FORMAT_PCM
	binary.LittleEndian.PutUint16(h[22:], uint16(channels))              //nolint:gosec // WAV header field width
	binary.LittleEndian.PutUint32(h[24:], uint32(sampleRate))            //nolint:gosec // WAV header field width
	binary.LittleEndian.PutUint32(h[28:], uint32(sampleRate*channels*2)) //nolint:gosec // WAV header field width
	binary.LittleEndian.PutUint16(h[32:], uint16(channels*2))            //nolint:gosec // WAV header field width
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], data)
	return h
}

// Init writes the WAV header.
func (ww *WAVWriter) Init(sampleRate, channels int, frameCount int64, bufferSize int) error {
	if channels <= 0 || sampleRate <= 0 {
		return errors.New("Invalid sample rate or number of channels")
	}
	ww.channels = channels
	ww.bw = bufio.NewWriterSize(ww.w, 1<<16)
	dataBytes := int64(-1)
	if ws, ok := ww.w.(io.WriteSeeker); ok {
		if pos, err := ws.Seek(0, io.SeekCurrent); err == nil {
			ww.seeker, ww.start = ws, pos
			dataBytes = 0 // patched in Done
		}
	}
	if ww.seeker == nil && frameCount > 0 {
		dataBytes = frameCount * int64(channels) * 2
	}
	_, ww.err = ww.bw.Write(wavHeader(sampleRate, channels, dataBytes))
	ww.buf = make([]byte, 0, 2*bufferSize)
	return ww.err
}

// ShouldContinue always reports true.
func (ww *WAVWriter) ShouldContinue() bool { return true }

// Process appends the samples as little-endian 16-bit PCM.
func (ww *WAVWriter) Process(samples []int16, frames int) error {
	if ww.err != nil {
		return ww.err
	}
	b := ww.buf[:0]
	for _, s := range samples[:frames*ww.channels] {
		b = binary.LittleEndian.AppendUint16(b, uint16(s)) //nolint:gosec // two's complement bits of a 16-bit sample
	}
	ww.buf = b
	if _, ww.err = ww.bw.Write(b); ww.err != nil {
		return ww.err
	}
	ww.frames += int64(frames)
	return nil
}

// Done flushes the data and, on a seekable destination, writes the final
// chunk sizes into the header. It returns the first write error, from Done
// or an earlier Process call.
func (ww *WAVWriter) Done() error {
	ww.finish()
	return ww.err
}

func (ww *WAVWriter) finish() {
	if ww.err != nil || ww.bw == nil {
		return
	}
	if ww.err = ww.bw.Flush(); ww.err != nil {
		return
	}
	ws := ww.seeker
	if ws == nil {
		return
	}
	dataBytes := ww.frames * int64(ww.channels) * 2
	h := wavHeader(0, 0, dataBytes)
	if _, ww.err = ws.Seek(ww.start+4, io.SeekStart); ww.err != nil {
		return
	}
	if _, ww.err = ws.Write(h[4:8]); ww.err != nil {
		return
	}
	if _, ww.err = ws.Seek(ww.start+40, io.SeekStart); ww.err != nil {
		return
	}
	if _, ww.err = ws.Write(h[40:44]); ww.err != nil {
		return
	}
	_, ww.err = ws.Seek(0, io.SeekEnd)
}

var _ audio.Processor = (*WAVWriter)(nil)
