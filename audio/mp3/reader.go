// Package mp3 decodes MPEG audio (layers I, II and III) the way the
// Mp3AudioFileReader of audiowaveform does, on top of a port of libmad.
package mp3

import (
	"errors"
	"fmt"
	"io"

	"github.com/rmens/audiowaveform-go/audio"
	"github.com/rmens/audiowaveform-go/internal/mad"
)

const (
	inputBufferSize  = 5 * 8192
	outputBufferSize = 8192

	// decoderDelay is the decoder delay added to the encoder delay of a
	// LAME or Lavc tag.
	decoderDelay = 529
)

var (
	magicXing = fourCC("Xing")
	magicInfo = fourCC("Info")
	magicLAME = fourCC("LAME")
	magicLavc = fourCC("Lavc")
)

// ErrStopped is returned when the processor's ShouldContinue reports false
// after Init. audiowaveform's MP3 reader treats that as a failed run.
var ErrStopped = errors.New("mp3: processor stopped after init")

func fourCC(s string) uint64 {
	return uint64(s[0])<<24 | uint64(s[1])<<16 | uint64(s[2])<<8 | uint64(s[3])
}

type reader struct {
	r io.Reader
}

// NewReader returns an audio.Reader that decodes the MPEG audio stream r.
func NewReader(r io.Reader) audio.Reader {
	return &reader{r: r}
}

// fixedToShort is MadFixedToShort: clip to +-32767, else truncate the
// fraction bits beyond 16-bit precision.
func fixedToShort(f int32) int16 {
	if f >= mad.FixedOne {
		return 32767
	}
	if f <= -mad.FixedOne {
		return -32767
	}
	return int16(f >> (mad.FracBits - 15))
}

// Run decodes the stream into p. Init is called when the first audio frame
// has been decoded, with frameCount 0; if no frame decodes, p is not used.
func (rd *reader) Run(p audio.Processor) error {
	var (
		stream mad.Stream
		frame  mad.Frame
		synth  mad.Synth
	)

	input := make([]byte, inputBufferSize+mad.BufferGuard)
	output := make([]int16, outputBufferSize)
	outPos := 0

	file := newBstdFile(rd.r)

	frameCount := 0
	samplesToSkip := 0
	first := true
	id3BytesToSkip := int64(0)
	channels := 0

	var runErr error
	processFailed := false

loop:
	for {
		if !stream.HasBuffer() || stream.Err == mad.ErrBufLen {
			var readStart, readSize, remaining int

			if stream.HasBuffer() {
				remaining = stream.BufEnd() - stream.NextFrame
				copy(input, input[stream.NextFrame:stream.BufEnd()])
				readStart = remaining
				readSize = inputBufferSize - remaining
			} else {
				readSize = inputBufferSize
			}

			// A frame that does not fit the buffer leaves no room to read
			// into; bstdfile then returns nothing and decoding ends.
			if readSize > 0 {
				readSize = file.read(input[readStart : readStart+readSize])
			}
			if readSize <= 0 {
				if file.err != nil {
					runErr = fmt.Errorf("mp3: read error on bit-stream: %w", file.err)
				}
				break
			}

			if first && readSize >= id3QuerySize {
				id3BytesToSkip = max(id3TagQuery(input[readStart:readStart+id3QuerySize]), 0)
				first = false
			}

			if id3BytesToSkip > int64(readSize) {
				id3BytesToSkip -= int64(readSize)
				continue
			} else if id3BytesToSkip > 0 {
				skip := int(id3BytesToSkip)
				readSize -= skip
				copy(input[readStart:], input[readStart+skip:readStart+skip+readSize])
				id3BytesToSkip = 0
			}

			if file.eof {
				guard := readStart + readSize
				clear(input[guard : guard+mad.BufferGuard])
				readSize += mad.BufferGuard
			}

			stream.SetBuffer(input, readSize+remaining)
			stream.Err = mad.ErrNone
		}

		if !frame.Decode(&stream) {
			if stream.Err.Recoverable() || stream.Err == mad.ErrBufLen {
				continue
			}
			runErr = fmt.Errorf("mp3: unrecoverable frame level error: %w", stream.Err)
			break
		}

		// The first frame may be a Xing/Info frame without audio. Its LAME or
		// Lavc tag gives the encoder delay, which is skipped together with the
		// decoder delay. The first audio frame sets the format.
		if frameCount == 0 {
			ptr := stream.AncPtr
			magic := ptr.Read(32)

			if magic == magicXing || magic == magicInfo {
				for range 116 {
					ptr.Read(8)
				}

				magic = ptr.Read(32)

				if magic == magicLAME || magic == magicLavc {
					for range 5 + 12 {
						ptr.Read(8)
					}
					delay := int(ptr.Read(12))
					samplesToSkip = decoderDelay + delay
				}
				continue
			}

			channels = frame.Header.Channels()

			if err := p.Init(int(frame.Header.SampleRate), channels, 0, outputBufferSize); err != nil {
				runErr = err
				processFailed = true
				break
			}
			if !p.ShouldContinue() {
				runErr = ErrStopped
				processFailed = true
				break
			}
		}

		frameCount++

		synth.Frame(&frame)

		stereo := frame.Header.Channels() == 2
		pcm := &synth.PCM

		for i := 0; i < pcm.Length; i++ {
			if samplesToSkip == 0 {
				if outPos < len(output) {
					output[outPos] = fixedToShort(pcm.Samples[0][i])
					outPos++
				}
				if stereo && outPos < len(output) {
					output[outPos] = fixedToShort(pcm.Samples[1][i])
					outPos++
				}
			} else {
				samplesToSkip--
			}

			if outPos == len(output) {
				frames := outputBufferSize / channels
				if err := p.Process(output[:frames*channels], frames); err != nil {
					// audiowaveform only leaves the sample loop here and
					// keeps decoding; stopping gives the same result.
					runErr = err
					processFailed = true
					break loop
				}
				outPos = 0
			}
		}
	}

	if outPos != 0 && !processFailed {
		frames := outPos / channels
		if err := p.Process(output[:frames*channels], frames); err != nil {
			runErr = err
		}
	}

	if frameCount > 0 {
		if err := p.Done(); runErr == nil {
			runErr = err
		}
	}

	return runErr
}
