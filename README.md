# audiowaveform-go

A Go port of [audiowaveform](https://codeberg.org/chrisn/audiowaveform) 1.11.1 by Chris Needham and BBC Research & Development.

It reads MP3, WAV, FLAC, Ogg Vorbis, Opus or raw audio and writes waveform data (.dat, .json, .txt), PNG images or WAV. It takes the same command line options as the C++ program and writes the same files. It is pure Go, so it builds to one binary without libmad, libsndfile, libgd or Boost.

## Install

    go install github.com/rmens/audiowaveform-go/cmd/audiowaveform@latest

## Use

The options are the same as upstream. See the [upstream documentation](https://codeberg.org/chrisn/audiowaveform#usage) or run `audiowaveform --help`.

    audiowaveform -i talk.mp3 -o talk.dat -z 256 -b 8
    audiowaveform -i talk.dat -o talk.png -w 1000 -h 200 -z auto
    ffmpeg -i video.mp4 -f wav - | audiowaveform --input-format wav --output-format dat -b 8 > video.dat

## Compatibility

The test suite runs this port and the C++ binary side by side on generated audio and compares the results.

- Waveform data from WAV, raw, FLAC and MP3 input is byte-identical. The MP3 decoder is a port of libmad 0.16.5, built the way Homebrew builds it (64-bit fixed point, accuracy mode), and gives the same samples. That covers MPEG layers I, II and III.
- PNG images are pixel-identical. The few libgd drawing routines upstream uses are ported, including its pie arcs for rounded bars and its small bitmap font.
- Ogg Vorbis and Opus are decoded in floating point, as libsndfile does, by different decoders. Samples can differ by 1 in the 16-bit range, in well under 1% of samples.
- WAV output to a file is byte-identical for FLAC, raw and MP3 input.

Where upstream crashes or loops forever, this port does not. Resampling to zero samples per pixel, an empty `--waveform-color` and a bar width plus gap above 2147483647 give an error. Auto amplitude scale with a start time past the end draws an image without a waveform. Axis labels stop where upstream's label loop would never end.

Other differences:

- FLAC, Ogg Vorbis and Opus can be read from a pipe. Upstream only supports MP3, WAV and raw there.
- WAV output can be written to a pipe. libsndfile refuses this.
- JSON waveform data with more than 24 channels is rejected, the same limit upstream applies to .dat files.
- WAV files with A-law, µ-law, ADPCM or GSM data are not supported. AIFF, CAF and other formats libsndfile detects from the file contents are not supported either.
- `--version` reports 1.11.1, the upstream release this port matches.

libmad can be built with other math settings. This port follows the Homebrew build. Other builds, such as Linux packages, were not tested and may differ by 1 in some MP3 samples.

## Library

The packages can also be used on their own:

- `waveform` holds the data type, the generator, rescaling and the .dat, .json and .txt formats.
- `audio/sndfile` and `audio/mp3` decode audio into an `audio.Processor`.
- `render` draws a waveform image.

## Tests

    go test ./...

To compare with the C++ program, install it and ffmpeg, then run:

    AUDIOWAVEFORM_REF=$(which audiowaveform) go test ./cmd/audiowaveform -run Compat

## License

GPL-3.0-or-later, like audiowaveform. See COPYING. The MP3 decoder is derived from libmad by Underbit Technologies and Robert Leslie (GPL-2.0-or-later). The renderer follows libgd. The Opus decoder is [gopus](https://github.com/thesyncim/gopus), the Vorbis decoder is [oggvorbis](https://github.com/jfreymuth/oggvorbis) and the FLAC decoder is [mewkiz/flac](https://github.com/mewkiz/flac).
