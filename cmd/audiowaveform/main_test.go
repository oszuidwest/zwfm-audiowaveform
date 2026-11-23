package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/oszuidwest/zwfm-audiowaveform/audio/sndfile"
)

func TestRunConvertsData(t *testing.T) {
	out := filepath.Join(t.TempDir(), "a.json")
	const base = "../../waveform/testdata/test_file_stereo_8bit_64spp_wav"
	if code := run([]string{"audiowaveform", "-q", "-i", base + ".dat", "-o", out, "-b", "8"}, nil, nil, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got, err := os.ReadFile(out) //nolint:gosec // reads test output
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

func TestRunFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := errors.Join(os.Mkdir(dir+"/d.mp3", 0o750), os.Mkdir(dir+"/d.json", 0o750), os.WriteFile(dir+"/ro.wav", nil, 0o600)); err != nil {
		t.Fatal(err)
	}
	roFile, _ := os.Open(dir + "/ro.wav") //nolint:gosec // test file in TempDir
	defer func() { _ = roFile.Close() }()
	toWAV := []string{"-i", "../../audio/mp3/testdata/stereo_l3.mp3", "-o", "-", "--output-format", "wav"}
	for _, tc := range []struct {
		args   []string
		stdout *os.File
		want   string
	}{
		{[]string{"-i", dir + "/d.mp3", "-o", dir + "/a.dat"}, nil, "\nRead error on bit-stream: Is a directory\n"},
		{[]string{"-i", dir + "/d.json", "-o", dir + "/a.dat"}, nil, "Failed to read file: " + dir + "/d.json\nIs a directory\n"},
		{toWAV, roFile, "System error : Bad file descriptor.\n"},
	} {
		var stderr bytes.Buffer
		if code := run(append([]string{"audiowaveform", "-q"}, tc.args...), nil, tc.stdout, &stderr); code != 1 || stderr.String() != tc.want {
			t.Errorf("%v: exit %d, stderr %q, want %q", tc.args, code, stderr.String(), tc.want)
		}
	}
}

// TestWAVWriteError checks the message for a write that fails after the
// header, which audiowaveform does not report.
func TestWAVWriteError(t *testing.T) {
	w := wavOutput{sndfile.NewWAVWriter(&failAfter{left: 44}), "-"}
	if err := w.Init(8000, 1, 0, 16384); err != nil {
		t.Fatal(err)
	}
	err := errors.Join(w.Process(make([]int16, 16384), 16384), w.Done())
	if want := "Failed to write WAV file: -\nBroken pipe"; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("got %v, want %q", err, want)
	}
}

// failAfter accepts left bytes, then fails like a closed pipe.
type failAfter struct{ left int }

func (f *failAfter) Write(p []byte) (int, error) {
	if len(p) > f.left {
		return 0, &fs.PathError{Op: "write", Path: "-", Err: syscall.EPIPE}
	}
	f.left -= len(p)
	return len(p), nil
}
