package main

import (
	"slices"
	"testing"
)

// replay records what an audioLoader sends back.
type replay struct {
	audioLoader
	first   *int16
	frames  []int
	samples []int16
}

func (r *replay) Process(samples []int16, frames int) error {
	if r.first == nil {
		r.first = &samples[0]
	}
	r.frames = append(r.frames, frames)
	r.samples = append(r.samples, samples...)
	return nil
}

// TestAudioLoaderKeepsBlocks checks that the stored samples never move while
// more arrive, and that the replay gives them back in blocks of
// 16384/channels frames.
func TestAudioLoaderKeepsBlocks(t *testing.T) {
	l := &audioLoader{}
	run := func() *replay {
		r := &replay{}
		if err := l.Run(r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	in := make([]int16, 3*12000)
	for i := range in {
		in[i] = int16(i)
	}
	_ = l.Init(8000, 3, 0, 0)
	_ = l.Process(in[:3*1000], 1000)
	before := run()
	_ = l.Process(in[3*1000:], 11000)
	after := run()
	if after.first != before.first {
		t.Error("stored samples were copied")
	}
	if want := []int{5461, 5461, 1078}; !slices.Equal(after.frames, want) {
		t.Errorf("replayed %v frames, want %v", after.frames, want)
	}
	if !slices.Equal(after.samples, in) {
		t.Error("replayed samples differ")
	}
}
