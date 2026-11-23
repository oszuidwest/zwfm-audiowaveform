// Package audio defines how decoded audio flows from a reader to a processor.
//
// A Reader decodes an input stream to interleaved signed 16-bit samples and
// pushes them, in blocks, to a Processor. This mirrors the AudioFileReader and
// AudioProcessor classes of audiowaveform.
package audio

// Processor consumes decoded audio.
type Processor interface {
	// Init is called once, before any samples. frameCount is the number of
	// frames the reader expects, or 0 if it does not know. bufferSize is the
	// largest number of samples (not frames) one Process call will carry.
	Init(sampleRate, channels int, frameCount int64, bufferSize int) error

	// ShouldContinue reports whether the processor wants samples after Init.
	// When it returns false the reader stops without calling Process or Done.
	ShouldContinue() bool

	// Process receives frames*channels interleaved samples.
	Process(samples []int16, frames int) error

	// Done is called once after the last Process call.
	Done()
}

// Reader decodes an audio stream into a Processor.
type Reader interface {
	Run(p Processor) error
}
