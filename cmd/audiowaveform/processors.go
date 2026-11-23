package main

import (
	"github.com/oszuidwest/zwfm-audiowaveform/audio"
	"github.com/oszuidwest/zwfm-audiowaveform/render"
	"github.com/oszuidwest/zwfm-audiowaveform/waveform"
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

func (d *durationCalculator) Done() error { return nil }

func (d *durationCalculator) duration() float64 {
	return float64(d.frames) / float64(d.sampleRate)
}

// audioLoader keeps all decoded samples in memory and can replay them. It
// stores them in blocks of the replay size, so growing never copies them.
type audioLoader struct {
	sampleRate, channels int
	frames               int
	blocks               [][]int16
}

// loaderBufferSize is the replay buffer size of VectorAudioFileReader.
const loaderBufferSize = 16384

func (l *audioLoader) Init(sampleRate, channels int, _ int64, _ int) error {
	l.sampleRate, l.channels = sampleRate, channels
	return nil
}

func (l *audioLoader) ShouldContinue() bool { return true }

func (l *audioLoader) Process(samples []int16, frames int) error {
	size := loaderBufferSize / l.channels * l.channels
	for s := samples[:frames*l.channels]; len(s) > 0; {
		if len(l.blocks) == 0 || len(l.blocks[len(l.blocks)-1]) == size {
			l.blocks = append(l.blocks, make([]int16, 0, size))
		}
		last := &l.blocks[len(l.blocks)-1]
		n := min(len(s), size-len(*last))
		*last = append(*last, s[:n]...)
		s = s[n:]
	}
	l.frames += frames
	return nil
}

func (l *audioLoader) Done() error { return nil }

func (l *audioLoader) duration() float64 {
	if l.channels == 0 {
		return 0 // nothing was decoded
	}
	return float64(l.frames) / float64(l.sampleRate)
}

// Run replays the samples in blocks of 16384/channels frames, like
// VectorAudioFileReader.
func (l *audioLoader) Run(p audio.Processor) error {
	if err := p.Init(l.sampleRate, l.channels, int64(l.frames), loaderBufferSize); err != nil || !p.ShouldContinue() {
		return err
	}
	for _, b := range l.blocks {
		if err := p.Process(b, len(b)/l.channels); err != nil {
			_ = p.Done()
			return err
		}
	}
	return p.Done()
}

// tracker wraps a processor to print status and progress messages.
type tracker struct {
	audio.Processor
	a          *app
	gen        *waveform.Generator
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
	t.sampleRate, t.total = sampleRate, frames
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
		t.a.infof("\rDone: %s", render.TimeLabel(seconds))
	}
}

func (t *tracker) Done() error {
	err := t.Processor.Done()
	if t.shown {
		t.a.infof("\n")
	}
	if t.gen != nil {
		t.a.infof("Generated %d points\n", t.gen.Points())
	}
	return err
}
