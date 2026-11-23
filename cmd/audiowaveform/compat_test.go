package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompat runs this port and the C++ audiowaveform side by side and
// compares exit codes, error messages and output files. It needs ffmpeg and
// AUDIOWAVEFORM_REF set to the reference binary.
func TestCompat(t *testing.T) {
	ref := os.Getenv("AUDIOWAVEFORM_REF")
	if ref == "" {
		t.Skip("set AUDIOWAVEFORM_REF to the C++ audiowaveform binary")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("needs ffmpeg")
	}
	dir := t.TempDir()
	ours := filepath.Join(dir, "audiowaveform-go")
	if out, err := exec.Command("go", "build", "-o", ours, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	in := filepath.Join(dir, "in")
	makeInputs(t, ref, in)

	const ogg, opus = 2, 2 // allowed int16 difference for float decoders
	cases := []struct {
		args  string
		tol   int
		stdin string // input file fed through a pipe
	}{
		// Audio to waveform data.
		{args: "-i ../in/mono.wav -o a.dat"},
		{args: "-i ../in/stereo.wav -o a.dat -b 8 -z 64"},
		{args: "-i ../in/stereo.wav -o a.json --split-channels -z 100"},
		{args: "-i ../in/multi.wav -o a.dat --split-channels -b 8 --pixels-per-second 30"},
		{args: "-i ../in/s24.wav -o a.dat -z 2 --split-channels"},
		{args: "-i ../in/f32.wav -o a.dat -z 2"},
		{args: "-i ../in/u8.wav -o a.dat -z 2"},
		{args: "-i ../in/stereo.flac -o a.dat -z 2 --split-channels"},
		{args: "-i ../in/s24.flac -o a.dat -z 2 --split-channels"},
		{args: "-i ../in/stereo.mp3 -o a.dat -z 2 --split-channels"},
		{args: "-i ../in/mono.mp3 -o a.dat -z 32"},
		{args: "-i ../in/noxing.mp3 -o a.dat -z 32 --split-channels"},
		{args: "-i ../in/layer2.mp3 -o a.dat -z 32 --split-channels"},
		{args: "-i ../in/stereo.ogg -o a.dat -z 64 --split-channels", tol: ogg},
		{args: "-i ../in/stereo.opus -o a.dat -z 64 --split-channels", tol: opus},
		{args: "-i ../in/stereo.raw --raw-format s16le --raw-channels 2 --raw-samplerate 44100 -o a.dat -z 2 --split-channels"},
		{args: "-i ../in/mono.raw --raw-format f32be --raw-channels 1 --raw-samplerate 8000 -o a.dat -z 2"},
		{args: "-i ../in/stereo.wav -o a.dat --amplitude-scale auto -b 8"},
		{args: "-i ../in/stereo.wav -o a.dat -s 0.5 -e 2.0 -w 333"},
		{args: "--input-format wav -o a.dat -z 50", stdin: "stereo.wav"},
		// Audio to WAV.
		{args: "-i ../in/stereo.mp3 -o a.wav"},
		{args: "-i ../in/stereo.flac -o a.wav"},
		{args: "-i ../in/stereo.ogg -o a.wav", tol: ogg},
		{args: "-i ../in/stereo.opus -o a.wav", tol: opus},
		{args: "-i ../in/stereo.raw --raw-format s16le --raw-channels 2 --raw-samplerate 44100 -o a.wav"},
		// Waveform data conversion and resampling.
		{args: "-i ../in/stereo.dat -o a.json"},
		{args: "-i ../in/stereo.dat -o a.txt -b 8"},
		{args: "-i ../in/mono.json -o a.dat"},
		{args: "-i ../in/stereo.dat -o a.dat -z 1000"},
		{args: "-i ../in/stereo.dat -o a.json --pixels-per-second 7 -b 8"},
		{args: "-i ../in/stereo8.dat -o a.dat -s 0.2 -e 1.5 -w 50"},
		// Images.
		{args: "-i ../in/stereo.dat -o a.png"},
		{args: "-i ../in/stereo.dat -o a.png -z auto -w 640 -h 120"},
		{args: "-i ../in/stereo.dat -o a.png -s 0.7 -z 128 --no-axis-labels"},
		{args: "-i ../in/stereo.dat -o a.png --waveform-style bars --bar-style rounded --bar-width 7 --bar-gap 2 -z auto"},
		{args: "-i ../in/stereo.dat -o a.png --waveform-style bars -z 200 --amplitude-scale auto -c audition"},
		{args: "-i ../in/mono.dat -o a.png -w 300 -h 100 --background-color ffffff00 --waveform-color ff000080 --border-color 00ff00 --axis-label-color 0000ffcc"},
		{args: "-i ../in/stereo.wav -o a.png --split-channels -z auto -w 500 --waveform-color ff0000,00ff00"},
		{args: "-i ../in/stereo.mp3 -o a.png -z auto"},
		{args: "-i ../in/long.json -o a.png -z auto -w 1200"},
		{args: "-i ../in/mono.wav -o a.png -s 1 -e 2 -w 400 --amplitude-scale 2.5"},
		{args: "--input-format wav -o a.png -z auto -w 300", stdin: "stereo.wav"},
		// Errors.
		{args: "--foo"},
		{args: "-i ../in/stereo.wav"},
		{args: "-i ../in/stereo.dat -o a.png --w 10"},
		{args: "-i ../in/stereo.dat -o a.png --waveform-style dots"},
		{args: "-i ../in/stereo.dat -o a.png -z 10"},
		{args: "-i ../in/stereo.wav -o a.dat -z 1"},
		{args: "-i ../in/stereo.wav -o a.dat -z 300 -e 2"},
		{args: "-i ../in/stereo.wav -o a.txt"},
		{args: "-i ../in/stereo.dat -o a.foo"},
		{args: "-i ../in/stereo.dat -o a.png --bits 12"},
		{args: "-i ../in/stereo.dat -o a.png --amplitude-scale -1"},
		{args: "-i ../in/stereo.dat -o a.png -c rainbow"},
		{args: "-i ../in/stereo.dat -o a.png --waveform-color ff0000,zz"},
		{args: "-i ../in/stereo.dat -o a.png -z 300 -z 400"},
		{args: "-i ../in/missing.dat -o a.json"},
		{args: "--width xx --width 2 --bits yy"},
		{args: "-xfoo"},
		{args: "--input-format json -o a.dat", stdin: "trailing.json"},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%02d %s", i, tc.args), func(t *testing.T) {
			run := func(bin, sub string) (int, string, string) {
				wd := filepath.Join(dir, fmt.Sprintf("%02d-%s", i, sub))
				os.MkdirAll(wd, 0o755)
				cmd := exec.Command(bin, append([]string{"-q"}, strings.Fields(tc.args)...)...)
				cmd.Args[0] = "audiowaveform"
				cmd.Dir = wd
				if tc.stdin != "" {
					b, _ := os.ReadFile(filepath.Join(in, tc.stdin))
					cmd.Stdin = bytes.NewReader(b) // a pipe, not a file
				}
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				cmd.Run()
				return cmd.ProcessState.ExitCode(), stderr.String(), wd
			}
			refCode, refErr, refDir := run(ref, "ref")
			code, errText, goDir := run(ours, "go")
			if code != refCode || errText != refErr {
				t.Fatalf("exit %d, stderr %q; reference: exit %d, stderr %q", code, errText, refCode, refErr)
			}
			files, _ := os.ReadDir(refDir)
			for _, f := range files {
				compareFiles(t, filepath.Join(refDir, f.Name()), filepath.Join(goDir, f.Name()), tc.tol)
			}
		})
	}
}

func makeInputs(t *testing.T, ref, in string) {
	t.Helper()
	os.MkdirAll(in, 0o755)
	ff := func(out string, args ...string) {
		args = append([]string{"-loglevel", "error", "-y"}, append(args, filepath.Join(in, out))...)
		if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg %s: %v\n%s", out, err, b)
		}
	}
	tone := "aevalsrc=0.6*sin(2*PI*220*t)*sin(PI*t/3)+0.3*random(0)-0.15:s=44100:d=3.3"
	stereo := "aevalsrc=0.9*sin(2*PI*440*t)|0.5*sin(2*PI*93*t)*cos(PI*t)+0.2*random(1)-0.1:s=48000:d=2.5"
	ff("mono.wav", "-f", "lavfi", "-i", tone)
	ff("stereo.wav", "-f", "lavfi", "-i", stereo)
	ff("multi.wav", "-f", "lavfi", "-i", tone, "-af", "pan=5.1|c0=c0|c1=0.5*c0|c2=0.2*c0|c3=c0|c4=0.7*c0|c5=0.9*c0", "-ar", "22050")
	ff("s24.wav", "-f", "lavfi", "-i", stereo, "-c:a", "pcm_s24le")
	ff("f32.wav", "-f", "lavfi", "-i", tone, "-c:a", "pcm_f32le")
	ff("u8.wav", "-f", "lavfi", "-i", tone, "-c:a", "pcm_u8")
	ff("stereo.flac", "-f", "lavfi", "-i", stereo)
	ff("s24.flac", "-f", "lavfi", "-i", stereo, "-sample_fmt", "s32", "-bits_per_raw_sample", "24")
	ff("stereo.mp3", "-f", "lavfi", "-i", stereo, "-ar", "44100", "-c:a", "libmp3lame", "-q:a", "4")
	ff("mono.mp3", "-f", "lavfi", "-i", tone, "-ar", "22050", "-c:a", "libmp3lame", "-b:a", "64k")
	ff("noxing.mp3", "-f", "lavfi", "-i", stereo, "-c:a", "libmp3lame", "-b:a", "160k", "-write_xing", "0")
	ff("layer2.mp3", "-f", "lavfi", "-i", stereo, "-c:a", "mp2", "-b:a", "192k", "-f", "mp2")
	ff("stereo.ogg", "-f", "lavfi", "-i", stereo, "-c:a", "vorbis", "-strict", "-2")
	ff("stereo.opus", "-f", "lavfi", "-i", stereo, "-c:a", "libopus")
	ff("stereo.raw", "-f", "lavfi", "-i", stereo, "-ar", "44100", "-f", "s16le")
	ff("mono.raw", "-f", "lavfi", "-i", tone, "-ar", "8000", "-f", "f32be")
	for _, args := range [][]string{
		{"-i", "stereo.wav", "-o", "stereo.dat", "-z", "64", "--split-channels"},
		{"-i", "stereo.wav", "-o", "stereo8.dat", "-z", "64", "--split-channels", "-b", "8"},
		{"-i", "mono.wav", "-o", "mono.dat", "-z", "64"},
		{"-i", "mono.wav", "-o", "mono.json", "-z", "128", "-b", "8"},
	} {
		cmd := exec.Command(ref, append([]string{"-q"}, args...)...)
		cmd.Dir = in
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, b)
		}
	}
	// 66 minutes at one point per second, for hh:mm:ss axis labels.
	var long strings.Builder
	long.WriteString(`{"version":2,"channels":1,"sample_rate":8000,"samples_per_pixel":8000,"bits":8,"length":3960,"data":[`)
	for i := range 3960 {
		if i > 0 {
			long.WriteString(",")
		}
		v := (i*37)%120 + 1
		fmt.Fprintf(&long, "%d,%d", -v, v)
	}
	long.WriteString("]}")
	os.WriteFile(filepath.Join(in, "long.json"), []byte(long.String()), 0o644)
	os.WriteFile(filepath.Join(in, "trailing.json"), []byte(`{"version":2,"length":1,"sample_rate":100,"samples_per_pixel":10,"data":[1,2]} x`), 0o644)
}

func compareFiles(t *testing.T, refPath, goPath string, tol int) {
	t.Helper()
	want, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatalf("missing output: %v", err)
	}
	switch {
	case bytes.Equal(got, want):
	case strings.HasSuffix(refPath, ".png"):
		comparePNG(t, want, got)
	case tol > 0 && (strings.HasSuffix(refPath, ".dat") || strings.HasSuffix(refPath, ".wav")):
		header := 44 // the WAV header written by libsndfile
		if strings.HasSuffix(refPath, ".dat") {
			header = 24
		}
		if len(got) != len(want) || !bytes.Equal(got[:header], want[:header]) {
			t.Fatalf("%s: size or header differs (%d vs %d bytes)", filepath.Base(refPath), len(got), len(want))
		}
		worst, count := 0, 0
		for i := header; i+1 < len(want); i += 2 {
			d := int(int16(binary.LittleEndian.Uint16(got[i:]))) - int(int16(binary.LittleEndian.Uint16(want[i:])))
			if d < 0 {
				d = -d
			}
			if d > 0 {
				count++
			}
			worst = max(worst, d)
		}
		if worst > tol {
			t.Errorf("%s: max difference %d over tolerance %d (%d values differ)", filepath.Base(refPath), worst, tol, count)
		}
	default:
		t.Errorf("%s differs", filepath.Base(refPath))
	}
}

func comparePNG(t *testing.T, want, got []byte) {
	t.Helper()
	a, err := png.Decode(bytes.NewReader(want))
	if err != nil {
		t.Fatal(err)
	}
	b, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if a.Bounds() != b.Bounds() {
		t.Fatalf("image size %v, reference %v", b.Bounds(), a.Bounds())
	}
	diff := 0
	var first image.Point
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if color.NRGBAModel.Convert(a.At(x, y)) != color.NRGBAModel.Convert(b.At(x, y)) {
				if diff == 0 {
					first = image.Pt(x, y)
				}
				diff++
			}
		}
	}
	if diff > 0 {
		t.Errorf("%d pixels differ, first at %v", diff, first)
	}
}
