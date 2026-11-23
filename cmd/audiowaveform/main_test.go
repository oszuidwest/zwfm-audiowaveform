package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRunConvertsData(t *testing.T) {
	out := filepath.Join(t.TempDir(), "a.json")
	const base = "../../waveform/testdata/test_file_stereo_8bit_64spp_wav"
	if code := run([]string{"audiowaveform", "-q", "-i", base + ".dat", "-o", out, "-b", "8"}, nil, nil, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := os.ReadFile(base + ".json"); !bytes.Equal(got, want) {
		t.Error("JSON output differs from audiowaveform's")
	}
	if code := run([]string{"audiowaveform", "-q", "-i", base + ".dat", "-o", out, "-b", "12"}, nil, nil, io.Discard); code != 1 {
		t.Errorf("-b 12: exit %d, want 1", code)
	}
}
