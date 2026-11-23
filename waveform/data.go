// Package waveform holds waveform data: per output point and channel, the
// minimum and maximum sample value of a block of audio samples.
package waveform

// MaxChannels is the largest number of channels a waveform can hold.
const MaxChannels = 24

// Data is waveform data. Samples holds, for each point and then each channel,
// a minimum and a maximum value in the signed 16-bit range.
type Data struct {
	SampleRate      int
	SamplesPerPixel int
	Channels        int
	Bits            int // resolution of the data as loaded: 8 or 16
	Samples         []int16
}

// New returns empty waveform data with one channel and 16-bit resolution.
func New() *Data {
	return &Data{Channels: 1, Bits: 16}
}

// Len returns the number of points.
func (d *Data) Len() int {
	return len(d.Samples) / (2 * d.Channels)
}

// Min returns the minimum value of a channel at point i.
func (d *Data) Min(channel, i int) int16 {
	return d.Samples[(i*d.Channels+channel)*2]
}

// Max returns the maximum value of a channel at point i.
func (d *Data) Max(channel, i int) int16 {
	return d.Samples[(i*d.Channels+channel)*2+1]
}

// Append adds a minimum and maximum value for the next channel of the
// current point.
func (d *Data) Append(lo, hi int16) {
	d.Samples = append(d.Samples, lo, hi)
}
