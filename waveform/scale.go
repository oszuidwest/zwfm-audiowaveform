package waveform

import "math"

// ScaleSample multiplies v by m, limits the result to the signed 16-bit range
// and truncates it toward zero.
func ScaleSample(v int, m float64) int16 {
	r := float64(v) * m
	if r > 32767 {
		r = 32767
	} else if r < -32768 {
		r = -32768
	}
	return int16(r)
}

// AmplitudeScale returns the factor that scales the loudest value of points
// start to end-1, over all channels, to full range. Like audiowaveform, it
// divides by the C int limits for an empty range; for start > end upstream
// reads out of bounds, so that range counts as empty too.
func AmplitudeScale(d *Data, start, end int) float64 {
	low, high := math.MaxInt32, math.MinInt32
	for i := start; i < end; i++ {
		for c := 0; c < d.Channels; c++ {
			low = min(low, int(d.Min(c, i)))
			high = max(high, int(d.Max(c, i)))
		}
	}
	scaleHigh, scaleLow := 1.0, 1.0
	if high != 0 {
		scaleHigh = 32767.0 / float64(high)
	}
	if low != 0 {
		scaleLow = 32767.0 / float64(low)
	}
	return math.Abs(math.Min(scaleHigh, scaleLow))
}

// ScaleAmplitude multiplies every value by scale, limited to 16 bits.
func ScaleAmplitude(d *Data, scale float64) {
	for i, v := range d.Samples {
		d.Samples[i] = ScaleSample(int(v), scale)
	}
}
