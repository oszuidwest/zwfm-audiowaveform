package mp3

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// recorder is an audio.Processor that keeps everything it receives.
type recorder struct {
	t          *testing.T
	sampleRate int
	channels   int
	inits      int
	dones      int
	samples    []int16
}

func (r *recorder) Init(sampleRate, channels int, frameCount int64, bufferSize int) error {
	r.inits++
	r.sampleRate, r.channels = sampleRate, channels
	if frameCount != 0 || bufferSize != outputBufferSize {
		r.t.Errorf("Init(%d, %d, %d, %d): want frameCount 0 and bufferSize %d",
			sampleRate, channels, frameCount, bufferSize, outputBufferSize)
	}
	return nil
}

func (r *recorder) ShouldContinue() bool { return true }

func (r *recorder) Process(samples []int16, frames int) error {
	if len(samples) != frames*r.channels || len(samples) > outputBufferSize {
		r.t.Errorf("Process got %d samples for %d frames", len(samples), frames)
	}
	r.samples = append(r.samples, samples...)
	return nil
}

func (r *recorder) Done() error { r.dones++; return nil }

// TestReaderMatchesReference decodes small fixtures and compares the PCM with
// the output of audiowaveform 1.11.1 (Homebrew, libmad 0.16.5): the SHA-256
// of the little-endian 16-bit samples it wrote with -o x.wav.
func TestReaderMatchesReference(t *testing.T) {
	tests := []struct {
		file       string
		sampleRate int
		channels   int
		samples    int
		sha256     string
	}{
		// MPEG-1 layer III, joint stereo, Xing/LAME tag (encoder delay skipped)
		{"stereo_l3.mp3", 44100, 2, 36958, "754ea7ba3cfa148a636bc311ab561d9c375865ab9b243715bb5f369d03db8fe1"},
		// MPEG-2 layer III, mono, behind an ID3v2 tag
		{"mono_l3_lsf_id3.mp3", 22050, 1, 9263, "bbae349afe3732d39524979840a6a9a87d874a3a364a9fec0536144adc933b5f"},
		// MPEG-1 layer II, stereo
		{"stereo_l2.mp3", 48000, 2, 29952, "fd779aad5318c15fcceccb90998da91a014500d594f694c1a97e082efc72549b"},
		// layer III frames with random content whose values overflow 32 bits
		// in the IMDCT, where the reference build does not wrap (see mulWide)
		{"crafted_l3_overflow.mp3", 44100, 2, 2304, "6af55ab316556f92a4b5ec550c786280122d859c16966e8921427fd3f54d8df6"},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}

			rec := &recorder{t: t}
			if err := NewReader(bytes.NewReader(data)).Run(rec); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if rec.inits != 1 || rec.dones != 1 {
				t.Errorf("Init called %d times, Done %d times; want 1 and 1", rec.inits, rec.dones)
			}
			if rec.sampleRate != tt.sampleRate || rec.channels != tt.channels {
				t.Errorf("got %d Hz, %d channels; want %d Hz, %d channels",
					rec.sampleRate, rec.channels, tt.sampleRate, tt.channels)
			}
			if len(rec.samples) != tt.samples {
				t.Errorf("got %d samples, want %d", len(rec.samples), tt.samples)
			}

			h := sha256.New()
			if err := binary.Write(h, binary.LittleEndian, rec.samples); err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(h.Sum(nil)); got != tt.sha256 {
				t.Errorf("PCM sha256 %s, want %s", got, tt.sha256)
			}
		})
	}
}

// TestReaderNoAudio checks that input without a decodable frame never
// reaches the processor, as in audiowaveform.
func TestReaderNoAudio(t *testing.T) {
	for _, input := range [][]byte{nil, []byte("ID3"), bytes.Repeat([]byte{0x55}, 5000)} {
		rec := &recorder{t: t}
		if err := NewReader(bytes.NewReader(input)).Run(rec); err != nil {
			t.Errorf("Run(%d bytes): %v", len(input), err)
		}
		if rec.inits != 0 || rec.dones != 0 || len(rec.samples) != 0 {
			t.Errorf("Run(%d bytes): Init %d, Done %d, %d samples; want none",
				len(input), rec.inits, rec.dones, len(rec.samples))
		}
	}
}

type failDone struct{ recorder }

func (*failDone) Done() error { return io.ErrShortWrite }

// TestReaderReportsDoneError checks that an error from Done reaches the
// caller of Run.
func TestReaderReportsDoneError(t *testing.T) {
	data, err := os.ReadFile("testdata/stereo_l2.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if err := NewReader(bytes.NewReader(data)).Run(&failDone{recorder{t: t}}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got %v", err)
	}
}
