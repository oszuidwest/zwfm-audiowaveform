package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/rmens/audiowaveform-go/audio"
	"github.com/rmens/audiowaveform-go/audio/mp3"
	"github.com/rmens/audiowaveform-go/audio/sndfile"
	"github.com/rmens/audiowaveform-go/render"
	"github.com/rmens/audiowaveform-go/waveform"
)

// app runs one command, like OptionHandler in audiowaveform.
type app struct {
	o      *options
	info   io.Writer // status messages, discarded with --quiet
	stdin  *os.File
	stdout *os.File
}

func (a *app) infof(format string, args ...any) {
	fmt.Fprintf(a.info, format, args...)
}

func isStdio(name string) bool { return name == "" || name == "-" }

func displayName(name, stdio string) string {
	if isStdio(name) {
		return stdio
	}
	return name
}

// osError formats a file error the way strerror does.
func osError(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	s := err.Error()
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (a *app) run() error {
	o := a.o
	in, out := o.inputFormat, o.outputFormat
	resampling := o.hasSamplesPerPixel || o.hasPixelsPerSecond || o.hasEnd
	switch {
	case in.isAudio() && in != formatWAV && out == formatWAV:
		return a.convertAudioFormat()
	case in.isAudio() && out.isWaveformData():
		return a.generateWaveformData()
	case in.isWaveformData() && !resampling && (out.isWaveformData() || out == formatTxt):
		return a.convertWaveformData()
	case (in.isWaveformData() || in.isAudio()) && out == formatPNG:
		return a.renderWaveformImage()
	case in.isWaveformData() && out.isWaveformData() && resampling:
		return a.resampleWaveformData()
	}
	// audiowaveform builds this message while converting the formats to
	// names, and an unknown format cuts it short.
	msg := "Can't generate "
	if out == formatUnknown {
		return errors.New(msg + "Unknown file format")
	}
	msg += out.String() + " format output from "
	if in == formatUnknown {
		return errors.New(msg + "Unknown file format")
	}
	return errors.New(msg + in.String() + " format input")
}

func (a *app) scaleFactor() (waveform.ScaleFactor, error) {
	o := a.o
	switch {
	case o.hasSamplesPerPixel && o.hasEnd:
		return nil, errors.New("Specify either --end or --zoom but not both")
	case o.hasPixelsPerSecond && o.hasEnd:
		return nil, errors.New("Specify either --end or --pixels-per-second but not both")
	case o.hasSamplesPerPixel && o.hasPixelsPerSecond:
		return nil, errors.New("Specify either --zoom or --pixels-per-second but not both")
	case o.hasEnd:
		return waveform.NewDuration(o.start, o.end, o.width)
	case o.hasPixelsPerSecond:
		return waveform.NewPixelsPerSecond(o.pixelsPerSecond)
	}
	return waveform.Zoom(o.samplesPerPixel), nil
}

// openAudio opens the input and returns a reader for its format.
func (a *app) openAudio() (audio.Reader, func(), error) {
	o := a.o
	f, closeFn := a.stdin, func() {}
	if !isStdio(o.input) {
		var err error
		if f, err = os.Open(o.input); err != nil {
			msg := osError(err)
			if o.inputFormat != formatMP3 {
				msg = "System error : " + msg + "." // libsndfile's wording
			}
			return nil, nil, fmt.Errorf("Failed to read file: %s\n%s", o.input, msg)
		}
		closeFn = func() { f.Close() }
	}
	a.infof("Input file: %s\n", displayName(o.input, "(stdin)"))
	src := io.Reader(f)
	if !isRegular(f) {
		src = struct{ io.Reader }{f} // hide Seek from pipes
	}
	var r audio.Reader
	var err error
	switch o.inputFormat {
	case formatMP3:
		r = mp3.NewReader(src)
	case formatRaw:
		r, err = sndfile.OpenRaw(src, o.rawChannels, o.rawSampleRate, o.rawFormat)
	default:
		if r, err = sndfile.Open(src); err != nil {
			if isStdio(o.input) {
				err = fmt.Errorf("Failed to read input: %w", err)
			} else {
				err = fmt.Errorf("Failed to read file: %s\n%w", o.input, err)
			}
		}
	}
	if err != nil {
		closeFn()
		return nil, nil, err
	}
	return r, closeFn, nil
}

// runAudio decodes the input into p.
func (a *app) runAudio(p audio.Processor) error {
	r, closeFn, err := a.openAudio()
	if err != nil {
		return err
	}
	defer closeFn()
	return r.Run(p)
}

func (a *app) convertAudioFormat() error {
	o := a.o
	r, closeFn, err := a.openAudio()
	if err != nil {
		return err
	}
	defer closeFn()
	a.infof("Output file: %s\n", displayName(o.output, "(stdout)"))
	if isStdio(o.output) {
		if isTerminal(a.stdout) {
			return errors.New("Cannot write WAV audio to the terminal")
		}
		w := io.Writer(a.stdout)
		if !isRegular(a.stdout) {
			w = struct{ io.Writer }{a.stdout} // libsndfile cannot seek in a pipe either
		}
		return runWAV(r, a.track, w)
	}
	out := &lazyFile{name: o.output}
	if err := runWAV(r, a.track, out); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// runWAV decodes r into a WAV writer and reports write errors that only
// show up when the writer finishes the header.
func runWAV(r audio.Reader, track func(audio.Processor, *waveform.Generator) audio.Processor, w io.Writer) error {
	ww := sndfile.NewWAVWriter(w)
	if err := r.Run(track(ww, nil)); err != nil {
		return err
	}
	return ww.Err()
}

func (a *app) generateWaveformData() error {
	o := a.o
	scale, err := a.scaleFactor()
	if err != nil {
		return err
	}
	d := waveform.New()
	gen := waveform.NewGenerator(d, scale, o.splitChannels)
	if err := a.runAudio(a.track(gen, gen)); err != nil {
		return err
	}
	if o.autoAmplitudeScale && d.Len() > 0 {
		waveform.ScaleAmplitude(d, waveform.AmplitudeScale(d, 0, d.Len()))
	}
	return a.writeData(d, o.outputFormat, o.bits)
}

func (a *app) loadWaveformData() (*waveform.Data, error) {
	o := a.o
	f := a.stdin
	if !isStdio(o.input) {
		var err error
		if f, err = os.Open(o.input); err != nil {
			if o.inputFormat == formatDat {
				return nil, fmt.Errorf("Failed to read data file: %s\n%s", o.input, osError(err))
			}
			return nil, fmt.Errorf("Failed to read file: %s\n%s", o.input, osError(err))
		}
		defer f.Close()
	}
	a.infof("Input file: %s\n", displayName(o.input, "(stdin)"))
	var d *waveform.Data
	var err error
	if o.inputFormat == formatDat {
		d, err = waveform.ReadDat(f)
		if errors.Is(err, waveform.ErrTruncated) && !isStdio(o.input) {
			err = nil // audiowaveform keeps what it read from a file
		}
		if err != nil {
			return nil, fmt.Errorf("Failed to read data file: %s\n%w", o.input, err)
		}
	} else if d, err = waveform.ReadJSON(f); err != nil {
		return nil, err
	}
	a.infof("Channels: %d\nSample rate: %d Hz\nBits: %d\nSamples per pixel: %d\nLength: %d points\n",
		d.Channels, d.SampleRate, d.Bits, d.SamplesPerPixel, d.Len())
	return d, nil
}

func (a *app) writeData(d *waveform.Data, f format, bits int) error {
	o := a.o
	err := a.writeOutput(func(w io.Writer) error {
		switch f {
		case formatDat:
			a.infof("Resolution: %d bits\nChannels: %d\n", bits, d.Channels)
			return d.WriteDat(w, bits)
		case formatJSON:
			return d.WriteJSON(w, bits)
		}
		return d.WriteText(w, bits)
	})
	if err != nil {
		return fmt.Errorf("Failed to write data file: %s\n%s", o.output, osError(err))
	}
	return nil
}

// writeOutput creates the output file (or uses stdout) and calls write.
func (a *app) writeOutput(write func(io.Writer) error) error {
	o := a.o
	w := io.Writer(a.stdout)
	var f *os.File
	if !isStdio(o.output) {
		var err error
		if f, err = os.Create(o.output); err != nil {
			return err
		}
		w = f
	}
	a.infof("Output file: %s\n", displayName(o.output, "(stdout)"))
	bw := bufio.NewWriter(w)
	err := write(bw)
	if err == nil {
		err = bw.Flush()
	}
	if f != nil {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

func (a *app) convertWaveformData() error {
	d, err := a.loadWaveformData()
	if err != nil {
		return err
	}
	bits := d.Bits
	if a.o.hasBits {
		bits = a.o.bits
	}
	return a.writeData(d, a.o.outputFormat, bits)
}

func (a *app) resampleWaveformData() error {
	d, err := a.loadWaveformData()
	if err != nil {
		return err
	}
	scale, err := a.scaleFactor()
	if err != nil {
		return err
	}
	samplesPerPixel := scale.SamplesPerPixel(d.SampleRate)
	// audiowaveform loops forever or divides by zero here.
	if samplesPerPixel < 1 || d.SamplesPerPixel < 1 {
		return fmt.Errorf("Invalid zoom: minimum 1")
	}
	a.infof("Rescaling to %d samples/pixel\n", samplesPerPixel)
	out := waveform.Rescale(d, samplesPerPixel)
	bits := d.Bits
	if a.o.hasBits {
		bits = a.o.bits
	}
	return a.writeData(out, a.o.outputFormat, bits)
}

func (a *app) colors() (render.Colors, error) {
	o := a.o
	var c render.Colors
	switch o.colorScheme {
	case "audacity":
		c = render.Audacity
	case "audition":
		c = render.Audition
	default:
		return c, fmt.Errorf("Unknown color scheme: %s", o.colorScheme)
	}
	if o.border != nil {
		c.Border = *o.border
	}
	if o.background != nil {
		c.Background = *o.background
	}
	if o.waveformColors != nil {
		c.Waveform = o.waveformColors
	}
	if o.axisLabel != nil {
		c.AxisLabel = *o.axisLabel
	}
	return c, nil
}

func (a *app) renderWaveformImage() error {
	o := a.o
	auto := o.autoSamplesPerPixel
	var scale waveform.ScaleFactor
	var err error
	if !auto {
		if scale, err = a.scaleFactor(); err != nil {
			return err
		}
	}
	colors, err := a.colors()
	if err != nil {
		return err
	}
	if o.start < 0 {
		return errors.New("Invalid start time: minimum 0")
	}
	bars := false
	switch o.waveformStyle {
	case "bars":
		bars = true
	case "normal":
	default:
		return fmt.Errorf("Unknown waveform style: %s", o.waveformStyle)
	}
	rounded := false
	if bars {
		switch o.barStyle {
		case "rounded":
			rounded = true
		case "square":
		default:
			return fmt.Errorf("Unknown waveform bar style: %s", o.barStyle)
		}
		if o.barWidth < 1 {
			return errors.New("Invalid bar width: minimum 1")
		}
		if o.barGap < 0 {
			return errors.New("Invalid bar gap: minimum 0")
		}
	}

	var d *waveform.Data
	var outSamplesPerPixel int
	if o.inputFormat.isWaveformData() {
		if d, err = a.loadWaveformData(); err != nil {
			return err
		}
		if auto {
			// Integer arithmetic, so whole seconds, as in audiowaveform.
			seconds := 0
			if d.SampleRate != 0 {
				seconds = d.Len() * d.SamplesPerPixel / d.SampleRate
			}
			if scale, err = waveform.NewDuration(0, float64(seconds), o.width); err != nil {
				return err
			}
		}
		outSamplesPerPixel = scale.SamplesPerPixel(d.SampleRate)
	} else {
		d = waveform.New()
		if isStdio(o.input) && !isSeekable(a.stdin) && auto {
			// A pipe cannot be read twice, so keep the audio in memory.
			loader := &audioLoader{}
			if err := a.runAudio(a.track(loader, nil)); err != nil {
				return err
			}
			if scale, err = waveform.NewDuration(0, loader.duration(), o.width); err != nil {
				return err
			}
			gen := waveform.NewGenerator(d, scale, o.splitChannels)
			if err := loader.Run(a.track(gen, gen)); err != nil {
				return err
			}
		} else {
			if auto {
				duration, err := a.audioDuration()
				if err != nil {
					return err
				}
				if scale, err = waveform.NewDuration(0, duration, o.width); err != nil {
					return err
				}
			}
			gen := waveform.NewGenerator(d, scale, o.splitChannels)
			if err := a.runAudio(a.track(gen, gen)); err != nil {
				return err
			}
		}
		outSamplesPerPixel = d.SamplesPerPixel
	}

	switch inSamplesPerPixel := d.SamplesPerPixel; {
	case outSamplesPerPixel == inSamplesPerPixel:
	case outSamplesPerPixel > inSamplesPerPixel && inSamplesPerPixel > 0:
		a.infof("Rescaling to %d samples/pixel\n", outSamplesPerPixel)
		d = waveform.Rescale(d, outSamplesPerPixel)
	default:
		return fmt.Errorf("Invalid zoom, minimum: %d", inSamplesPerPixel)
	}

	img, err := render.Render(d, render.Options{
		Width:              o.width,
		Height:             o.height,
		StartTime:          o.start,
		AxisLabels:         o.axisLabels,
		Bars:               bars,
		BarWidth:           o.barWidth,
		BarGap:             o.barGap,
		RoundedBars:        rounded,
		AutoAmplitudeScale: o.autoAmplitudeScale,
		AmplitudeScale:     o.amplitudeScale,
		Colors:             colors,
	})
	if err != nil {
		return err
	}
	a.infof("Image dimensions: %dx%d pixels\n", o.width, o.height)
	err = a.writeOutput(func(w io.Writer) error {
		return render.EncodePNG(w, img, colors.HasAlpha(), o.compression)
	})
	if err != nil {
		return fmt.Errorf("Failed to write PNG file: %s\n%s", o.output, osError(err))
	}
	return nil
}

// audioDuration decodes the input once to find its length in seconds.
func (a *app) audioDuration() (float64, error) {
	calc := &durationCalculator{}
	a.infof("Calculating audio duration...\n")
	if err := a.runAudio(a.track(calc, nil)); err != nil {
		return 0, err
	}
	duration := calc.duration()
	a.infof("Duration: %.6g seconds\n", duration)
	if isStdio(a.o.input) {
		if _, err := a.stdin.Seek(0, io.SeekStart); err != nil {
			return 0, errors.New("Failed to seek to start of audio")
		}
	}
	return duration, nil
}

func isSeekable(f *os.File) bool {
	st, err := f.Stat()
	return err != nil || st.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0
}

func isRegular(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode().IsRegular()
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// lazyFile creates the file on first use, as libsndfile opens the output
// only once the audio format is known.
type lazyFile struct {
	name string
	f    *os.File
	err  error
}

func (l *lazyFile) open() error {
	if l.f == nil && l.err == nil {
		if l.f, l.err = os.Create(l.name); l.err != nil {
			l.err = fmt.Errorf("Failed to write file: %s\n%s", l.name, osError(l.err))
		}
	}
	return l.err
}

func (l *lazyFile) Write(p []byte) (int, error) {
	if err := l.open(); err != nil {
		return 0, err
	}
	return l.f.Write(p)
}

func (l *lazyFile) Seek(offset int64, whence int) (int64, error) {
	if err := l.open(); err != nil {
		return 0, err
	}
	return l.f.Seek(offset, whence)
}

func (l *lazyFile) Close() error {
	if l.f == nil {
		return nil
	}
	return l.f.Close()
}
