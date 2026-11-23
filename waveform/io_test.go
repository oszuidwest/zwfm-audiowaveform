package waveform

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

// TestFormatsMatchUpstreamFixtures checks the readers and writers byte for
// byte against audiowaveform's own test files in testdata.
func TestFormatsMatchUpstreamFixtures(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile("testdata/" + name) //nolint:gosec // reads testdata
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	const base = "test_file_stereo_8bit_64spp_wav"
	fromDat, err := ReadDat(bytes.NewReader(read(base + ".dat")))
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := ReadJSON(bytes.NewReader(read(base + ".json")))
	if err != nil {
		t.Fatal(err)
	}
	twoChannels, err := ReadDat(bytes.NewReader(read("test_file_2channel_8bit_64spp_wav.dat")))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		data  *Data
		write func(*Data, *bytes.Buffer) error
		want  string
	}{
		{"dat to json", fromDat, func(d *Data, b *bytes.Buffer) error { return d.WriteJSON(b, 8) }, base + ".json"},
		{"dat to txt", fromDat, func(d *Data, b *bytes.Buffer) error { return d.WriteText(b, 8) }, base + ".txt"},
		{"json to dat", fromJSON, func(d *Data, b *bytes.Buffer) error { return d.WriteDat(b, 8) }, base + ".dat"},
		{"two channel dat", twoChannels, func(d *Data, b *bytes.Buffer) error { return d.WriteDat(b, 8) }, "test_file_2channel_8bit_64spp_wav.dat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := tt.write(tt.data, &got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), read(tt.want)) {
				t.Errorf("output differs from %s", tt.want)
			}
		})
	}
}

func TestReadJSONRejectsTooManyChannels(t *testing.T) {
	in := `{"version":2,"channels":25,"sample_rate":8000,"samples_per_pixel":256,"bits":8,"length":0,"data":[]}`
	if _, err := ReadJSON(strings.NewReader(in)); err == nil || err.Error() != "Invalid channels: 25" {
		t.Fatalf("got %v", err)
	}
}

// TestGenerator uses hand-computed values: the mono mix averages the
// channels, and the last frame makes a partial point.
func TestGenerator(t *testing.T) {
	for split, want := range map[bool][]int16{false: {-5, 35, 5, 5}, true: {10, 30, -20, 40, 5, 5, 5, 5}} {
		d := New()
		g := NewGenerator(d, Zoom(2), split)
		if err := g.Init(8000, 2, 0, 0); err != nil {
			t.Fatal(err)
		}
		if err := g.Process([]int16{10, -20, 30, 40, 5, 5}, 3); err != nil {
			t.Fatal(err)
		}
		if err := g.Done(); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(d.Samples, want) {
			t.Errorf("split=%v: %v, want %v", split, d.Samples, want)
		}
	}
}

func TestDat16AndRescale(t *testing.T) {
	b, err := os.ReadFile("testdata/test_file_stereo_8bit_64spp_wav.dat")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ReadDat(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := d.WriteDat(&buf, 16); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadDat(&buf); err != nil || !slices.Equal(got.Samples, d.Samples) {
		t.Errorf("16-bit round trip: %v", err)
	}
	buf.Reset()
	if err := Rescale(d, 128).WriteDat(&buf, 8); err != nil {
		t.Fatal(err)
	}
	// Written by audiowaveform: -i test_file_stereo_8bit_64spp_wav.dat -z 128.
	if want, _ := os.ReadFile("../render/testdata/stereo_128spp.dat"); !bytes.Equal(buf.Bytes(), want) {
		t.Error("Rescale to 128 samples per pixel differs from audiowaveform")
	}
}

func TestReadJSONStreams(t *testing.T) {
	valid := `{"version":2,"length":0,"sample_rate":8000,"samples_per_pixel":256,"data":[]}`
	// Data after the object is read but not kept, and a read error still wins.
	padding := strings.Repeat(" ", 8<<20)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := ReadJSON(io.MultiReader(strings.NewReader(valid), strings.NewReader(padding), iotest.ErrReader(io.ErrUnexpectedEOF)))
	runtime.ReadMemStats(&after)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("got %v, want the read error", err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Errorf("allocated %d bytes for an empty waveform", allocated)
	}
	// After a read error the source is not asked again, as it may block.
	src := &failingReader{}
	if _, err := ReadJSON(src); err == nil || src.calls != 1 {
		t.Errorf("got %v after %d reads, want an error after 1", err, src.calls)
	}
	// Error positions count every line read, however the reads are split.
	for in, line := range map[string]string{
		`{"version":2,` + strings.Repeat("\n", 5000) + "x" + strings.Repeat("\n", 50) + "}": "line 5001,",
		`{"ver\uson":2}`: "line 1,",
	} {
		_, whole := ReadJSON(strings.NewReader(in))
		_, split := ReadJSON(iotest.OneByteReader(strings.NewReader(in)))
		if fmt.Sprint(split) != fmt.Sprint(whole) || !strings.Contains(fmt.Sprint(whole), line) {
			t.Errorf("%.20q: got %v and %v, want %s", in, whole, split, line)
		}
	}
}

type failingReader struct{ calls int }

func (r *failingReader) Read([]byte) (int, error) {
	r.calls++
	return 0, io.ErrClosedPipe
}
