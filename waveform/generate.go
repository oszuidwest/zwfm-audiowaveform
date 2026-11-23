package waveform

import (
	"errors"
	"fmt"
	"math"
)

// ScaleFactor decides how many audio samples make one waveform point.
type ScaleFactor interface {
	SamplesPerPixel(sampleRate int) int
}

// Zoom is a fixed number of samples per point.
type Zoom int

// SamplesPerPixel returns z.
func (z Zoom) SamplesPerPixel(int) int { return int(z) }

// PixelsPerSecond is a fixed number of points per second of audio.
type PixelsPerSecond int

// SamplesPerPixel returns sampleRate / p, rounded down.
func (p PixelsPerSecond) SamplesPerPixel(sampleRate int) int { return sampleRate / int(p) }

// NewPixelsPerSecond checks that p is positive.
func NewPixelsPerSecond(p int) (PixelsPerSecond, error) {
	if p <= 0 {
		return 0, errors.New("Invalid pixels per second: must be greater than zero")
	}
	return PixelsPerSecond(p), nil
}

// Duration fits the time from Start to End seconds into Width points.
type Duration struct {
	Start, End float64
	Width      int
}

// NewDuration checks the time range and width.
func NewDuration(start, end float64, width int) (Duration, error) {
	if end < start {
		return Duration{}, fmt.Errorf("Invalid end time, must be greater than %.6g", start)
	}
	if width < 1 {
		return Duration{}, errors.New("Invalid image width: minimum 1")
	}
	return Duration{start, end, width}, nil
}

// SamplesPerPixel returns the number of samples per point that fits the
// range into the width, rounded down.
func (d Duration) SamplesPerPixel(sampleRate int) int {
	samples := int64((d.End - d.Start) * float64(sampleRate))
	return int(samples / int64(d.Width))
}

// Generator builds waveform data from audio. It implements audio.Processor.
// See BlockFile::CalcSummary in Audacity.
type Generator struct {
	data            *Data
	scale           ScaleFactor
	split           bool
	channels        int
	outChannels     int
	samplesPerPixel int
	count           int
	lo, hi          []int
}

// NewGenerator returns a generator that writes into d. With split set, each
// audio channel gets its own waveform; otherwise channels are averaged.
func NewGenerator(d *Data, scale ScaleFactor, split bool) *Generator {
	return &Generator{data: d, scale: scale, split: split}
}

// Init sets up the generator for the audio format.
func (g *Generator) Init(sampleRate, channels int, _ int64, _ int) error {
	if channels < 1 || channels > MaxChannels {
		return fmt.Errorf("Cannot generate waveform data from audio file with %d channels", channels)
	}
	g.channels = channels
	g.samplesPerPixel = g.scale.SamplesPerPixel(sampleRate)
	if g.samplesPerPixel < 2 {
		return errors.New("Invalid zoom: minimum 2")
	}
	g.outChannels = 1
	if g.split {
		g.outChannels = channels
	}
	g.data.SamplesPerPixel = g.samplesPerPixel
	g.data.SampleRate = sampleRate
	g.data.Channels = g.outChannels
	g.lo = make([]int, g.outChannels)
	g.hi = make([]int, g.outChannels)
	g.reset()
	return nil
}

// SamplesPerPixel returns the zoom chosen in Init.
func (g *Generator) SamplesPerPixel() int { return g.samplesPerPixel }

// Points returns the number of points generated so far.
func (g *Generator) Points() int { return g.data.Len() }

// OutputChannels returns the number of waveform channels chosen in Init.
func (g *Generator) OutputChannels() int { return g.outChannels }

// ShouldContinue always returns true.
func (g *Generator) ShouldContinue() bool { return true }

func (g *Generator) reset() {
	for c := range g.lo {
		g.lo[c] = math.MaxInt16
		g.hi[c] = math.MinInt16
	}
	g.count = 0
}

func (g *Generator) flush() {
	for c := range g.lo {
		g.data.Append(int16(g.lo[c]), int16(g.hi[c]))
	}
	g.reset()
}

// Process adds interleaved samples.
func (g *Generator) Process(samples []int16, frames int) error {
	for i := range frames {
		frame := samples[i*g.channels : (i+1)*g.channels]
		if g.outChannels == 1 {
			sum := 0
			for _, s := range frame {
				sum += int(s)
			}
			s := sum / g.channels
			g.lo[0] = min(g.lo[0], s)
			g.hi[0] = max(g.hi[0], s)
		} else {
			for c, s := range frame {
				g.lo[c] = min(g.lo[c], int(s))
				g.hi[c] = max(g.hi[c], int(s))
			}
		}
		g.count++
		if g.count == g.samplesPerPixel {
			g.flush()
		}
	}
	return nil
}

// Done adds the last, partly filled point.
func (g *Generator) Done() {
	if g.count > 0 {
		g.flush()
	}
}
