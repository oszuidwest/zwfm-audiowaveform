package main

import (
	"fmt"

	"github.com/rmens/audiowaveform-go/audio"
	"github.com/rmens/audiowaveform-go/waveform"
)

// durationCalculator finds the length of the audio. It stops the reader
// right after Init when the reader already knows the frame count.
type durationCalculator struct {
	sampleRate int
	frames     int64
}

func (d *durationCalculator) Init(sampleRate, _ int, frames int64, _ int) error {
	d.sampleRate, d.frames = sampleRate, frames
	return nil
}

func (d *durationCalculator) ShouldContinue() bool { return d.frames == 0 }

func (d *durationCalculator) Process(_ []int16, frames int) error {
	d.frames += int64(frames)
	return nil
}

func (d *durationCalculator) Done() {}

func (d *durationCalculator) duration() float64 {
	return float64(d.frames) / float64(d.sampleRate)
}

// audioLoader keeps all decoded samples in memory and can replay them.
type audioLoader struct {
	sampleRate, channels int
	samples              []int16
}

func (l *audioLoader) Init(sampleRate, channels int, _ int64, _ int) error {
	l.sampleRate, l.channels = sampleRate, channels
	return nil
}

func (l *audioLoader) ShouldContinue() bool { return true }

func (l *audioLoader) Process(samples []int16, frames int) error {
	l.samples = append(l.samples, samples[:frames*l.channels]...)
	return nil
}

func (l *audioLoader) Done() {}

func (l *audioLoader) duration() float64 {
	return float64(len(l.samples)/l.channels) / float64(l.sampleRate)
}

// Run replays the samples, like VectorAudioFileReader.
func (l *audioLoader) Run(p audio.Processor) error {
	const bufferSize = 16384
	total := len(l.samples) / l.channels
	if err := p.Init(l.sampleRate, l.channels, int64(total), bufferSize); err != nil || !p.ShouldContinue() {
		return err
	}
	step := bufferSize / l.channels
	for i := 0; i < total; i += step {
		n := min(step, total-i)
		if err := p.Process(l.samples[i*l.channels:(i+n)*l.channels], n); err != nil {
			return err
		}
	}
	p.Done()
	return nil
}

// tracker wraps a processor to print status and progress messages.
type tracker struct {
	audio.Processor
	a          *app
	gen        *waveform.Generator
	channels   int
	sampleRate int
	total      int64
	done       int64
	percent    int
	seconds    int
	shown      bool
}

func (a *app) track(p audio.Processor, gen *waveform.Generator) audio.Processor {
	return &tracker{Processor: p, a: a, gen: gen, percent: -1, seconds: -1}
}

func (t *tracker) Init(sampleRate, channels int, frames int64, bufferSize int) error {
	if err := t.Processor.Init(sampleRate, channels, frames, bufferSize); err != nil {
		return err
	}
	t.sampleRate, t.channels, t.total = sampleRate, channels, frames
	if t.gen != nil {
		t.a.infof("Generating waveform data...\nSamples per pixel: %d\nInput channels: %d\nOutput channels: %d\n",
			t.gen.SamplesPerPixel(), channels, t.gen.OutputChannels())
	}
	return nil
}

func (t *tracker) Process(samples []int16, frames int) error {
	if !t.shown {
		t.progress()
	}
	if err := t.Processor.Process(samples, frames); err != nil {
		return err
	}
	t.done += int64(frames)
	t.progress()
	return nil
}

// progress prints the share done when the length is known, else the time.
func (t *tracker) progress() {
	t.shown = true
	if t.total > 0 {
		percent := int(min(t.done*100/t.total, 100))
		if percent != t.percent {
			t.percent = percent
			t.a.infof("\rDone: %d%%", percent)
		}
		return
	}
	seconds := int(t.done / int64(max(t.sampleRate, 1)))
	if seconds != t.seconds {
		t.seconds = seconds
		t.a.infof("\rDone: %s", formatTime(seconds))
	}
}

func (t *tracker) Done() {
	t.Processor.Done()
	if t.shown {
		t.a.infof("\n")
	}
	if t.gen != nil {
		t.a.infof("Generated %d points\n", t.gen.Points())
	}
}

func formatTime(seconds int) string {
	h, m, s := seconds/3600, seconds/60%60, seconds%60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
