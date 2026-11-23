package main

import (
	"bytes"
	"errors"
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
	// 4 MB of raw audio gives more WAV output than any pipe buffer holds, so
	// the write after the header always fails once the reader is gone.
	if err := errors.Join(os.Mkdir(dir+"/d.mp3", 0o750), os.Mkdir(dir+"/d.json", 0o750), os.WriteFile(dir+"/ro.wav", nil, 0o600),
		os.WriteFile(dir+"/in.raw", make([]byte, 4<<20), 0o600)); err != nil {
		t.Fatal(err)
	}
	roFile, _ := os.Open(dir + "/ro.wav") //nolint:gosec // test file in TempDir
	pr, pw, _ := os.Pipe()
	defer func() { _ = errors.Join(roFile.Close(), pw.Close()) }()
	go func() { _, _ = io.ReadFull(pr, make([]byte, 44)); _ = pr.Close() }() // take the header only
	toWAV := []string{"-i", dir + "/in.raw", "--raw-format", "s16le", "--raw-channels", "1", "--raw-samplerate", "8000", "-o", "-", "--output-format", "wav"}
	for _, tc := range []struct {
		args   []string
		stdout *os.File
		want   string
	}{
		{[]string{"-i", dir + "/d.mp3", "-o", dir + "/a.dat"}, nil, "\nRead error on bit-stream: Is a directory\n"},
		{[]string{"-i", dir + "/d.json", "-o", dir + "/a.dat"}, nil, "Failed to read file: " + dir + "/d.json\nIs a directory\n"},
		{toWAV, roFile, "System error : Bad file descriptor.\n"},
		{toWAV, pw, "Failed to write WAV file: -\nBroken pipe\n"},
	} {
		var stderr bytes.Buffer
		if code := run(append([]string{"audiowaveform", "-q"}, tc.args...), nil, tc.stdout, &stderr); code != 1 || stderr.String() != tc.want {
			t.Errorf("%v: exit %d, stderr %q, want %q", tc.args, code, stderr.String(), tc.want)
		}
	}
}
