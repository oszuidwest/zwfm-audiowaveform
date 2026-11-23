package sndfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rmens/audiowaveform-go/audio"
)

// capture records everything a reader sends.
type capture struct {
	rate, channels int
	frames         int64
	calls          []int
	samples        []int16
	done           int
	stop           bool
}

func (c *capture) Init(rate, ch int, frames int64, _ int) error {
	c.rate, c.channels, c.frames = rate, ch, frames
	return nil
}
func (c *capture) ShouldContinue() bool { return !c.stop }
func (c *capture) Process(s []int16, n int) error {
	c.calls = append(c.calls, n)
	c.samples = append(c.samples, s[:n*c.channels]...)
	return nil
}
func (c *capture) Done() error { c.done++; return nil }

func runFn(t *testing.T) func(audio.Reader, error) *capture {
	return func(r audio.Reader, err error) *capture { return run(t, r, err) }
}

func run(t *testing.T, r audio.Reader, err error) *capture {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	c := &capture{}
	if err := r.Run(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func riff(id string, fmtChunk, data []byte, order binary.AppendByteOrder) []byte {
	b := order.AppendUint32([]byte(id), uint32(4+8+len(fmtChunk)+8+len(data)))
	b = append(b, "WAVEfmt "...)
	b = order.AppendUint32(b, uint32(len(fmtChunk)))
	b = append(b, fmtChunk...)
	b = append(b, "data"...)
	b = order.AppendUint32(b, uint32(len(data)))
	return append(b, data...)
}

func fmtChunk(order binary.ByteOrder, format, bits int) []byte {
	b := make([]byte, 16)
	order.PutUint16(b[0:], uint16(format))
	order.PutUint16(b[2:], 1)
	order.PutUint32(b[4:], 8000)
	order.PutUint32(b[8:], uint32(8000*(bits+7)/8))
	order.PutUint16(b[12:], uint16((bits+7)/8))
	order.PutUint16(b[14:], uint16(bits))
	return b
}

func extensible(bits int, sub uint16) []byte {
	b := append(fmtChunk(binary.LittleEndian, waveFormatExtensible, bits), make([]byte, 24)...)
	binary.LittleEndian.PutUint16(b[16:], 22)
	binary.LittleEndian.PutUint16(b[18:], uint16(bits))
	binary.LittleEndian.PutUint32(b[20:], 4) // front centre
	binary.LittleEndian.PutUint16(b[24:], sub)
	copy(b[26:], guidTail)
	return b
}

func f32le(v ...float32) []byte {
	var b []byte
	for _, f := range v {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(f))
	}
	return b
}

func f64be(v ...float64) []byte {
	var b []byte
	for _, f := range v {
		b = binary.BigEndian.AppendUint64(b, math.Float64bits(f))
	}
	return b
}

// TestConversions checks the 16-bit conversion of each PCM encoding. The
// expected values come from audiowaveform 1.11.1 with libsndfile 1.2.2 on arm64.
func TestConversions(t *testing.T) {
	le, be := binary.LittleEndian, binary.BigEndian
	nan := float32(math.NaN())
	floatIn := []float32{1.5, -1.5, 2, 1e10, -1e10, nan, 0.5, -0.5, 1, -1, -0.00001}
	floatOut := []int16{-16386, 16386, -2, -1, 0, 0, 16383, -16383, 32767, -32767, 0}
	s24 := []byte{0xff, 0xff, 0x7f, 0x00, 0x00, 0x80, 0xff, 0x00, 0x00, 0x00, 0xff, 0xff}
	tests := []struct {
		name string
		file []byte
		raw  string // OpenRaw format, or "" for Open
		want []int16
	}{
		{"wav u8", riff("RIFF", fmtChunk(le, 1, 8), []byte{0x00, 0x80, 0xff}, le), "", []int16{-32768, 0, 32512}},
		{"wav s16", riff("RIFF", fmtChunk(le, 1, 16), []byte{0x01, 0x80, 0xff, 0x7f}, le), "", []int16{-32767, 32767}},
		{"rifx s16", riff("RIFX", fmtChunk(be, 1, 16), []byte{0x80, 0x01, 0x7f, 0xff}, be), "", []int16{-32767, 32767}},
		{"wav s24", riff("RIFF", fmtChunk(le, 1, 24), s24, le), "", []int16{32767, -32768, 0, -1}},
		{"wav 20 bit in 24", riff("RIFF", fmtChunk(le, 1, 20), s24, le), "", []int16{32767, -32768, 0, -1}},
		{"wav s32", riff("RIFF", fmtChunk(le, 1, 32), []byte{0xff, 0xff, 0xff, 0x7f, 0, 0, 0, 0x80, 0xff, 0xff, 0, 0}, le), "", []int16{32767, -32768, 0}},
		{"wav f32", riff("RIFF", fmtChunk(le, 3, 32), f32le(floatIn...), le), "", floatOut},
		{"wav f64 over full scale", riff("RIFF", fmtChunk(le, 3, 64), binary.LittleEndian.AppendUint64(nil, math.Float64bits(3)), le), "", []int16{32765}},
		{"extensible s24", riff("RIFF", extensible(24, 1), s24, le), "", []int16{32767, -32768, 0, -1}},
		{"extensible f32", riff("RIFF", extensible(32, 3), f32le(floatIn...), le), "", floatOut},
		{"raw s8", []byte{0x80, 0x00, 0x7f}, "s8", []int16{-32768, 0, 32512}},
		{"raw u8", []byte{0x00, 0x80, 0xff}, "u8", []int16{-32768, 0, 32512}},
		{"raw s16be", []byte{0x80, 0x01, 0x7f, 0xff}, "s16be", []int16{-32767, 32767}},
		{"raw s24be", []byte{0x7f, 0xff, 0xff, 0x80, 0x00, 0x00}, "s24be", []int16{32767, -32768}},
		{"raw s32be", []byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, "s32be", []int16{32767, -1}},
		{"raw f32le", f32le(floatIn...), "f32le", floatOut},
		{"raw f64be", f64be(1.5, -0.5), "f64be", []int16{-16386, -16383}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r audio.Reader
			var err error
			if tt.raw != "" {
				r, err = OpenRaw(bytes.NewReader(tt.file), 1, 8000, tt.raw)
			} else {
				r, err = Open(bytes.NewReader(tt.file))
			}
			c := run(t, r, err)
			if !slices.Equal(c.samples, tt.want) {
				t.Errorf("samples = %v, want %v", c.samples, tt.want)
			}
			if c.frames != int64(len(tt.want)) || c.rate != 8000 || c.channels != 1 {
				t.Errorf("Init(%d, %d, %d), want (8000, 1, %d)", c.rate, c.channels, c.frames, len(tt.want))
			}
		})
	}
}

// onlyReader hides io.Seeker, like a pipe.
type onlyReader struct{ r *bytes.Reader }

func (o onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

// TestWAVUnknownSizeFromPipe checks that 0xffffffff chunk sizes read to the
// end of the input, with frame count 0 from a pipe and the real count when the
// input can seek.
func TestWAVUnknownSizeFromPipe(t *testing.T) {
	data := []byte{1, 0, 2, 0, 3, 0}
	file := riff("RIFF", fmtChunk(binary.LittleEndian, 1, 16), data, binary.LittleEndian)
	binary.LittleEndian.PutUint32(file[4:], 0xffffffff)
	binary.LittleEndian.PutUint32(file[40:], 0xffffffff)
	c := runFn(t)(Open(onlyReader{bytes.NewReader(file)}))
	if c.frames != 0 || !slices.Equal(c.samples, []int16{1, 2, 3}) {
		t.Errorf("frames %d samples %v", c.frames, c.samples)
	}
	c = runFn(t)(Open(bytes.NewReader(file))) // seekable: length from the file
	if c.frames != 3 {
		t.Errorf("seekable frames = %d, want 3", c.frames)
	}
}

// TestRunBlocks checks that Run reads blocks of 16384/channels frames and,
// like upstream, passes the final short (possibly empty) block before Done.
func TestRunBlocks(t *testing.T) {
	raw := make([]byte, 2730*2*6*2)
	c := runFn(t)(OpenRaw(bytes.NewReader(raw), 6, 44100, "s16le"))
	if !slices.Equal(c.calls, []int{2730, 2730, 0}) || c.done != 1 {
		t.Errorf("calls %v done %d", c.calls, c.done)
	}
	r, _ := OpenRaw(bytes.NewReader(raw), 6, 44100, "s16le")
	c = &capture{stop: true}
	if err := r.Run(c); err != nil || len(c.calls) != 0 || c.done != 0 {
		t.Errorf("stopped run: err %v calls %v done %d", err, c.calls, c.done)
	}
}

func TestOpenRawErrors(t *testing.T) {
	for _, tt := range []struct {
		ch, rate int
		format   string
		want     string
	}{
		{0, 44100, "s16le", "Invalid number of input channels: must be greater than zero"},
		{1, 0, "s16le", "Invalid input sample rate: must be greater than zero"},
		{1, 44100, "s16", "Unsupported format: s16"},
	} {
		if _, err := OpenRaw(bytes.NewReader(nil), tt.ch, tt.rate, tt.format); err == nil || err.Error() != tt.want {
			t.Errorf("OpenRaw(%d, %d, %q) error %v, want %q", tt.ch, tt.rate, tt.format, err, tt.want)
		}
	}
}

// TestFixtures decodes each fixture from a seekable and a non-seekable input.
// The FLAC hash is the hash of the reference binary's WAV output. Vorbis
// and Opus are decoded in float, whose last bit depends on how the compiler
// fuses the operations (it differs under -race and between architectures), so
// they get a sum of all samples with a small tolerance instead; the reference
// comparison in cmd/audiowaveform checks them sample by sample.
func TestFixtures(t *testing.T) {
	for _, tt := range []struct {
		file           string
		rate, channels int
		frames         int64
		sha            string // "" for float decoders
		sum            int64  // sum of all samples, for float decoders
	}{
		{"id3_s24.flac", 48000, 2, 4800, "13886f89da6be2c123beb6f8477d8a32cce167473b8e444e8784a0ff24ded8a0", 0},
		{"mono16k.opus", 16000, 1, 16000, "", -389115},
		{"stereo_over.opus", 48000, 2, 24000, "", 195062},
		{"stereo.oga", 16000, 2, 113519, "", 783667},
	} {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			for _, seekable := range []bool{true, false} {
				var c *capture
				if seekable {
					c = runFn(t)(Open(bytes.NewReader(data)))
				} else {
					c = runFn(t)(Open(onlyReader{bytes.NewReader(data)}))
				}
				pcm := make([]byte, 0, 2*len(c.samples))
				for _, s := range c.samples {
					pcm = binary.LittleEndian.AppendUint16(pcm, uint16(s))
				}
				sum := sha256.Sum256(pcm)
				if got := hex.EncodeToString(sum[:]); tt.sha != "" && got != tt.sha {
					t.Errorf("seekable=%v: sha256 %s, want %s", seekable, got, tt.sha)
				}
				var total int64
				for _, s := range c.samples {
					total += int64(s)
				}
				if tt.sha == "" && (total < tt.sum-64 || total > tt.sum+64) {
					t.Errorf("seekable=%v: sample sum %d, want %d±64", seekable, total, tt.sum)
				}
				if c.rate != tt.rate || c.channels != tt.channels || int64(len(c.samples)/c.channels) != tt.frames {
					t.Errorf("seekable=%v: %d Hz %d ch %d frames", seekable, c.rate, c.channels, len(c.samples)/c.channels)
				}
				if seekable && c.frames != tt.frames {
					t.Errorf("Init frame count %d, want %d", c.frames, tt.frames)
				}
			}
		})
	}
}

// libsndfile's header for SF_FORMAT_WAV | SF_FORMAT_PCM_16, 44.1 kHz mono,
// 22050 frames, as written by audiowaveform -o x.wav.
const refHeader = "52494646" + "68ac0000" + "57415645" + "666d7420" + "10000000" +
	"0100" + "0100" + "44ac0000" + "88580100" + "0200" + "1000" + "64617461" + "44ac0000"

// TestWAVWriter checks the header against refHeader on a file and on a pipe.
func TestWAVWriter(t *testing.T) {
	samples := make([]int16, 16384)
	for i := range samples {
		samples[i] = int16(i * 7)
	}
	write := func(p audio.Processor, frameCount int64) {
		if err := p.Init(44100, 1, frameCount, 16384); err != nil {
			t.Fatal(err)
		}
		for _, n := range []int{16384, 5666} {
			if err := p.Process(samples, n); err != nil {
				t.Fatal(err)
			}
		}
		if err := p.Done(); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(t.TempDir(), "out.wav")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	write(NewWAVWriter(f), 0)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if h := hex.EncodeToString(got[:44]); h != refHeader {
		t.Errorf("file header %s, want %s", h, refHeader)
	}
	if len(got) != 44+22050*2 || binary.LittleEndian.Uint16(got[44+2*3:]) != 21 {
		t.Errorf("file length %d or data wrong", len(got))
	}

	// A pipe: the header carries the frame count from Init, or 0xffffffff.
	var buf bytes.Buffer
	write(NewWAVWriter(&buf), 22050)
	if !bytes.Equal(buf.Bytes(), got) {
		t.Error("pipe output with known frame count differs from file output")
	}
	buf.Reset()
	write(NewWAVWriter(&buf), 0)
	if binary.LittleEndian.Uint32(buf.Bytes()[4:]) != 0xffffffff || binary.LittleEndian.Uint32(buf.Bytes()[40:]) != 0xffffffff {
		t.Errorf("pipe header with unknown length: % x", buf.Bytes()[:44])
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrShortWrite }

// TestRunReportsDoneError checks that a write error surfacing in Done reaches
// the caller of Run.
func TestRunReportsDoneError(t *testing.T) {
	r, err := OpenRaw(bytes.NewReader(make([]byte, 4)), 1, 8000, "s16le")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(NewWAVWriter(failWriter{})); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got %v", err)
	}
}
