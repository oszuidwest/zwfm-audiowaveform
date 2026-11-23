# zwfm-audiowaveform

Generate waveform data and render waveform images from audio files, in pure Go.

This is a Go port of [audiowaveform](https://codeberg.org/chrisn/audiowaveform) by Chris Needham. It reads MP3, WAV, FLAC, Ogg Vorbis, Opus and raw audio, and writes waveform data as binary (.dat), JSON or text, PNG images, or WAV. It takes the same command line options as audiowaveform and gives the same waveform data and images. It works as a drop-in replacement, for example as the data source for [Peaks.js](https://codeberg.org/chrisn/peaks.js).

It builds to a single binary with no C libraries, and can also be used as a Go library.

## Install

Needs Go 1.27.3 or later.

```sh
go install github.com/oszuidwest/zwfm-audiowaveform/cmd/audiowaveform@latest
```

## Usage

Create 8-bit waveform data at 256 samples per point:

```sh
audiowaveform -i episode.mp3 -o episode.dat -z 256 -b 8
```

Render a 1000x200 image, with the zoom picked to fit the width:

```sh
audiowaveform -i episode.dat -o episode.png -w 1000 -h 200 -z auto
```

Render rounded bars, one waveform per channel:

```sh
audiowaveform -i episode.wav -o episode.png -z auto --split-channels \
  --waveform-style bars --bar-style rounded --bar-width 6 --bar-gap 2
```

Read from a pipe and write to stdout:

```sh
ffmpeg -i video.mp4 -f wav - | audiowaveform --input-format wav --output-format json -b 8 > video.json
```

Run `audiowaveform --help` for all options. The [audiowaveform documentation](https://codeberg.org/chrisn/audiowaveform#usage) describes each of them, and the [data format](https://codeberg.org/chrisn/audiowaveform/src/branch/master/doc/DataFormat.md) of the .dat and .json files.

## Library

```go
in, err := os.Open("episode.mp3")
if err != nil {
	return err
}
defer in.Close()

data := waveform.New()
gen := waveform.NewGenerator(data, waveform.Zoom(256), false)
if err := mp3.NewReader(in).Run(gen); err != nil {
	return err
}
if err := data.WriteJSON(out, 8); err != nil {
	return err
}

img, err := render.Render(data, &render.Options{
	Width: 1000, Height: 200, AxisLabels: true,
	AmplitudeScale: 1, Colors: render.Audacity,
})
```

The packages are:

- `waveform`: waveform data, generation, rescaling, and the .dat, .json and .txt formats.
- `audio/mp3`: MPEG audio layers I, II and III.
- `audio/sndfile`: WAV, RF64, Wave64, FLAC, Ogg Vorbis, Ogg Opus and raw PCM, plus a WAV writer.
- `render`: waveform images and PNG encoding.

## Compatibility

The output matches audiowaveform 1.11.1. A test runs both programs side by side on a set of generated files and compares the results. Waveform data from WAV, FLAC, raw and MP3 input is byte-identical. Images have the same pixels, though the PNG files are compressed differently. Vorbis and Opus are decoded in floating point, so a few samples can differ by one step.

To run that test, install audiowaveform and ffmpeg and run:

```sh
AUDIOWAVEFORM_REF=$(which audiowaveform) go test ./cmd/audiowaveform -run Compat
```

A few things work differently on purpose:

- FLAC, Ogg Vorbis and Opus can be read from a pipe, and WAV can be written to one.
- Input that makes audiowaveform crash or hang gives an error or a finished image here.
- Some file errors give the real system error, or a message where audiowaveform prints none.
- WAV files with A-law, µ-law, ADPCM or GSM audio are not supported, and neither are AIFF and CAF.

## Credits

audiowaveform was written by Chris Needham at BBC Research & Development and is maintained on [Codeberg](https://codeberg.org/chrisn/audiowaveform). This port follows its design and behaviour closely. Thanks to Chris and all audiowaveform contributors.

Waveform generation and rescaling use algorithms adapted from [Audacity](https://www.audacityteam.org), as in audiowaveform. The MP3 decoder is a port of [libmad](https://codeberg.org/tenacityteam/libmad) by Robert Leslie and Underbit Technologies. The image drawing follows [libgd](https://libgd.github.io). Opus, Vorbis and FLAC decoding use [gopus](https://github.com/thesyncim/gopus), [oggvorbis](https://github.com/jfreymuth/oggvorbis) and [flac](https://github.com/mewkiz/flac).

## License

GPL-3.0-or-later, the same as audiowaveform. See [COPYING](COPYING).
