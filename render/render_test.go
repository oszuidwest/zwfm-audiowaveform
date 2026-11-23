package render

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/rmens/audiowaveform-go/waveform"
)

// The PNG files in testdata come from audiowaveform's own test data. The
// 128spp .dat files were made from upstream's 64spp ones with the reference
// binary (-z 128), which renders them to the same pixels as upstream's tests.

func readDat(t *testing.T, name string) *waveform.Data {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	d, err := waveform.ReadDat(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func readPNG(t *testing.T, r *bytes.Reader) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(r)
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, image.Point{}, draw.Src)
	return out
}

func samePixels(t *testing.T, want, got *image.NRGBA) {
	t.Helper()
	if want.Bounds() != got.Bounds() {
		t.Fatalf("size %v, want %v", got.Bounds(), want.Bounds())
	}
	for i := 0; i < len(want.Pix); i += 4 {
		if !bytes.Equal(want.Pix[i:i+4], got.Pix[i:i+4]) {
			p, w := i/4, want.Rect.Dx()
			t.Fatalf("pixel (%d,%d) = %v, want %v", p%w, p/w, got.Pix[i:i+4], want.Pix[i:i+4])
		}
	}
}

func TestRenderMatchesUpstream(t *testing.T) {
	std := Options{Width: 800, Height: 250, AxisLabels: true, AmplitudeScale: 1, Colors: Audacity, BarWidth: 8, BarGap: 4}
	with := func(f func(o *Options)) Options {
		o := std
		f(&o)
		return o
	}
	tests := []struct {
		dat, png string
		o        Options
	}{
		{"test_file_image_amplitude_scale_1channel.dat", "test_file_image_amplitude_scale_1channel.png", std},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_audition.png", with(func(o *Options) { o.Colors = Audition })},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_colors.png", with(func(o *Options) {
			o.Colors = Colors{
				Border:     color.NRGBA{255, 0, 0, 255},
				Background: color.NRGBA{0, 255, 0, 0x80},
				Waveform:   []color.NRGBA{{0, 0, 255, 255}},
				AxisLabel:  color.NRGBA{255, 255, 255, 255},
			}
		})},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_no_axis_labels.png", with(func(o *Options) { o.AxisLabels = false })},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_scale_1.5.png", with(func(o *Options) { o.AmplitudeScale = 1.5 })},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_scale_auto.png", with(func(o *Options) { o.AutoAmplitudeScale = true })},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_offset.png", with(func(o *Options) { o.StartTime = 0.5 })},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_square_bars.png", with(func(o *Options) { o.Bars = true })},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_square_bars_offset.png", with(func(o *Options) { o.Bars, o.StartTime = true, 0.5 })},
		{"stereo_128spp.dat", "test_file_stereo_dat_128spp_rounded_bars.png", with(func(o *Options) { o.Bars, o.RoundedBars = true, true })},
		{"2channel_128spp.dat", "test_file_2channel_8bit_64spp_wav.png", std},
		{"2channel_128spp.dat", "test_file_2channel_8bit_64spp_wav_bars.png", with(func(o *Options) { o.Bars = true })},
	}
	for _, tt := range tests {
		t.Run(tt.png, func(t *testing.T) {
			ref, err := os.ReadFile(filepath.Join("testdata", tt.png))
			if err != nil {
				t.Fatal(err)
			}
			want := readPNG(t, bytes.NewReader(ref))
			got, err := Render(readDat(t, tt.dat), tt.o)
			if err != nil {
				t.Fatal(err)
			}
			samePixels(t, want, got)

			// The encoded image must decode to the same pixels, with alpha
			// exactly when upstream writes it.
			var buf bytes.Buffer
			if err := EncodePNG(&buf, got, tt.o.Colors.HasAlpha(), 9); err != nil {
				t.Fatal(err)
			}
			if buf.Bytes()[25] != ref[25] {
				t.Errorf("PNG color type %d, want %d", buf.Bytes()[25], ref[25])
			}
			samePixels(t, want, readPNG(t, bytes.NewReader(buf.Bytes())))
		})
	}
}

func TestRenderErrors(t *testing.T) {
	d := &waveform.Data{SampleRate: 44100, SamplesPerPixel: 64, Channels: 1, Samples: []int16{-1, 1}}
	ok := Options{Width: 10, Height: 10, AmplitudeScale: 1, Colors: Audacity}
	tests := []struct {
		name string
		d    *waveform.Data
		mod  func(o *Options)
		want string
	}{
		{"width", d, func(o *Options) { o.Width = 0 }, "Invalid image width: minimum 1"},
		{"height", d, func(o *Options) { o.Height = 0 }, "Invalid image height: minimum 1"},
		{"start", d, func(o *Options) { o.StartTime = -1 }, "Invalid start time: minimum 0"},
		{"bar width", d, func(o *Options) { o.Bars, o.BarWidth = true, 0 }, "Invalid bar width: minimum 1"},
		{"bar gap", d, func(o *Options) { o.Bars, o.BarWidth, o.BarGap = true, 1, -1 }, "Invalid bar gap: minimum 0"},
		{"bar size", d, func(o *Options) { o.Bars, o.BarWidth, o.BarGap = true, math.MaxInt, 1 }, "Invalid bar width and gap: too large"},
		{"empty", &waveform.Data{SampleRate: 44100, SamplesPerPixel: 64, Channels: 1}, func(*Options) {}, "Empty waveform buffer"},
		{"sample rate", &waveform.Data{SamplesPerPixel: 64, Channels: 1, Samples: []int16{0, 0}}, func(*Options) {}, "Invalid sample rate: 0 Hz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := ok
			tt.mod(&o)
			if _, err := Render(tt.d, o); err == nil || err.Error() != tt.want {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTrigTables(t *testing.T) {
	// Spot values from libgd's gdtables.c, including the exact halves that
	// plain truncation of the floating-point product gets wrong.
	for _, c := range []struct{ deg, cos, sin int }{
		{0, 1024, 0}, {1, 1023, 17}, {30, 886, 512}, {45, 724, 724},
		{120, -512, 886}, {150, -886, 512}, {181, -1023, -17}, {270, 0, -1024}, {359, 1023, -17},
	} {
		if gdCosT[c.deg] != c.cos || gdSinT[c.deg] != c.sin {
			t.Errorf("%d degrees: cos %d sin %d, want %d %d", c.deg, gdCosT[c.deg], gdSinT[c.deg], c.cos, c.sin)
		}
	}
}

// TestRenderEndsOnAbsurdZoom covers a zoom where upstream's axis label loop
// never ends.
func TestRenderEndsOnAbsurdZoom(t *testing.T) {
	d := &waveform.Data{SampleRate: 1, SamplesPerPixel: math.MaxInt32, Channels: 1, Bits: 16, Samples: []int16{-100, 100}}
	if _, err := Render(d, Options{Width: 100, Height: 50, AxisLabels: true, AmplitudeScale: 1, Colors: Audacity}); err != nil {
		t.Fatal(err)
	}
}
