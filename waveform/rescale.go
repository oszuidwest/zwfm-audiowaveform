package waveform

import "math"

// Rescale returns in at a coarser zoom of samplesPerPixel samples per point.
// See Sequence::GetWaveDisplay in Audacity.
func Rescale(in *Data, samplesPerPixel int) *Data {
	out := &Data{
		SampleRate:      in.SampleRate,
		SamplesPerPixel: samplesPerPixel,
		Channels:        in.Channels,
		Bits:            16,
	}
	inPerPixel := in.SamplesPerPixel
	size := in.Len()
	at := func(x int) int { return x * samplesPerPixel }

	lo := make([]int16, in.Channels)
	hi := make([]int16, in.Channels)
	if size > 0 {
		for c := range lo {
			lo[c], hi[c] = in.Min(c, 0), in.Max(c, 0)
		}
	}

	input, output, lastInput := 0, 0, 0
	for input < size {
		for at(output)/inPerPixel == input {
			if output > 0 {
				for c := range lo {
					out.Append(lo[c], hi[c])
				}
			}
			lastInput = input
			output++
			if at(output) != at(output-1) {
				for c := range lo {
					lo[c], hi[c] = math.MaxInt16, math.MinInt16
				}
			}
		}
		stop := min(at(output)/inPerPixel, size)
		for ; input < stop; input++ {
			for c := range lo {
				lo[c] = min(lo[c], in.Min(c, input))
				hi[c] = max(hi[c], in.Max(c, input))
			}
		}
	}
	if input != lastInput {
		for c := range lo {
			out.Append(lo[c], hi[c])
		}
	}
	return out
}
