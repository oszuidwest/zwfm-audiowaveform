package main

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"image/color"
)

// option is one command line option, parsed like boost::program_options
// does by default: --name value, --name=value, -n value, -nvalue, grouped
// short switches (-qv), and unique prefixes of long names (--zo).
type option struct {
	long  string
	short byte
	kind  byte // 0 for a switch, else 's'tring, 'i'nt, 'f'loat or 'c'olor
	seen  bool
	value string
}

type options struct {
	table []*option

	help, version, quiet, splitChannels, axisLabels bool

	input, output             string
	inputFormat, outputFormat format
	hasInputFormat            bool
	hasOutputFormat           bool

	samplesPerPixel     int
	autoSamplesPerPixel bool
	hasSamplesPerPixel  bool
	pixelsPerSecond     int
	hasPixelsPerSecond  bool
	bits                int
	hasBits             bool
	start, end          float64
	hasEnd              bool

	width, height                  int
	colorScheme                    string
	border, background, axisLabel  *color.NRGBA
	waveformColors                 []color.NRGBA
	waveformStyle, barStyle        string
	barWidth, barGap               int
	autoAmplitudeScale             bool
	amplitudeScale                 float64
	compression                    int
	rawSampleRate, rawChannels     int
	rawFormat                      string
	hasRawSampleRate, hasRawChanls bool
	hasRawFormat                   bool
}

// optionError is a command line error; it is reported with a hint to --help.
type optionError struct{ msg string }

func (e optionError) Error() string { return e.msg }

func optErrorf(format string, a ...any) error {
	return optionError{fmt.Sprintf(format, a...)}
}

func newOptions() *options {
	o := &options{}
	add := func(long string, short byte, kind byte) {
		o.table = append(o.table, &option{long: long, short: short, kind: kind})
	}
	add("help", 0, 0)
	add("version", 'v', 0)
	add("quiet", 'q', 0)
	add("input-filename", 'i', 's')
	add("output-filename", 'o', 's')
	add("split-channels", 0, 0)
	add("input-format", 0, 's')
	add("output-format", 0, 's')
	add("zoom", 'z', 's')
	add("pixels-per-second", 0, 'i')
	add("bits", 'b', 'i')
	add("start", 's', 'f')
	add("end", 'e', 'f')
	add("width", 'w', 'i')
	add("height", 'h', 'i')
	add("colors", 'c', 's')
	add("border-color", 0, 'c')
	add("background-color", 0, 'c')
	add("waveform-color", 0, 's')
	add("waveform-style", 0, 's')
	add("bar-width", 0, 'i')
	add("bar-gap", 0, 'i')
	add("bar-style", 0, 's')
	add("axis-label-color", 0, 'c')
	add("no-axis-labels", 0, 0)
	add("with-axis-labels", 0, 0)
	add("amplitude-scale", 0, 's')
	add("compression", 0, 'i')
	add("raw-samplerate", 0, 'i')
	add("raw-channels", 0, 'i')
	add("raw-format", 0, 's')
	return o
}

func (o *options) get(long string) *option {
	for _, opt := range o.table {
		if opt.long == long {
			return opt
		}
	}
	panic("unknown option " + long)
}

func (o *options) findLong(name, token string) (*option, error) {
	var matches []*option
	for _, opt := range o.table {
		if opt.long == name {
			return opt, nil
		}
		if strings.HasPrefix(opt.long, name) {
			matches = append(matches, opt)
		}
	}
	switch len(matches) {
	case 0:
		return nil, optErrorf("unrecognised option '%s'", token)
	case 1:
		return matches[0], nil
	}
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = "'--" + m.long + "'"
	}
	slices.Sort(names)
	list := strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
	return nil, optErrorf("option '%s' is ambiguous and matches %s", token, list)
}

func (o *options) findShort(c byte, token string) (*option, error) {
	for _, opt := range o.table {
		if opt.short == c {
			return opt, nil
		}
	}
	return nil, optErrorf("unrecognised option '%s'", token)
}

// tokenize assigns the arguments to options. Other arguments are ignored,
// as boost does without a positional options description.
func (o *options) tokenize(args []string) error {
	type occurrence struct {
		opt   *option
		value string
	}
	var seen []occurrence
	set := func(opt *option, value string) {
		seen = append(seen, occurrence{opt, value})
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			i = len(args)
		case strings.HasPrefix(arg, "--"):
			name, value, hasEq := strings.Cut(arg[2:], "=")
			opt, err := o.findLong(name, arg)
			if err != nil {
				return err
			}
			switch {
			case opt.kind == 0 && hasEq:
				return optErrorf("option '--%s' does not take any arguments", opt.long)
			case opt.kind == 0:
				set(opt, "")
			case hasEq && value == "":
				return optErrorf("the argument for option '--%s' should follow immediately after the equal sign", opt.long)
			case hasEq:
				set(opt, value)
			case i+1 < len(args):
				i++
				set(opt, args[i])
			default:
				return optErrorf("the required argument for option '--%s' is missing", opt.long)
			}
		case len(arg) > 1 && arg[0] == '-':
			for j := 1; j < len(arg); j++ {
				opt, err := o.findShort(arg[j], arg)
				if err != nil {
					return err
				}
				if opt.kind == 0 {
					set(opt, "")
					continue
				}
				switch {
				case j+1 < len(arg):
					set(opt, arg[j+1:])
				case i+1 < len(args):
					i++
					set(opt, args[i])
				default:
					return optErrorf("the required argument for option '--%s' is missing", opt.long)
				}
				break
			}
		}
	}
	// Like boost, check each value in command line order.
	for _, oc := range seen {
		if oc.opt.seen {
			return optErrorf("option '--%s' cannot be specified more than once", oc.opt.long)
		}
		oc.opt.seen, oc.opt.value = true, oc.value
		if err := oc.opt.check(); err != nil {
			return err
		}
	}
	return nil
}

// check validates the value of a typed option.
func (opt *option) check() error {
	ok := true
	switch opt.kind {
	case 'i':
		_, err := strconv.ParseInt(opt.value, 10, 32)
		ok = err == nil
	case 'f':
		_, err := strconv.ParseFloat(opt.value, 64)
		ok = err == nil && !strings.ContainsAny(opt.value, "_xXpP")
	case 'c':
		if _, ok = parseColor(opt.value); !ok {
			return optionError{"Invalid color value"}
		}
	}
	if !ok {
		return optErrorf("the argument ('%s') for option '--%s' is invalid", opt.value, opt.long)
	}
	return nil
}

func (o *options) str(long, def string) string {
	if opt := o.get(long); opt.seen {
		return opt.value
	}
	return def
}

func (o *options) int(long string, def int) int {
	opt := o.get(long)
	if !opt.seen {
		return def
	}
	n, _ := strconv.Atoi(opt.value)
	return n
}

func (o *options) float(long string, def float64) float64 {
	opt := o.get(long)
	if !opt.seen {
		return def
	}
	f, _ := strconv.ParseFloat(opt.value, 64)
	return f
}

func (o *options) color(long string) *color.NRGBA {
	opt := o.get(long)
	if !opt.seen {
		return nil
	}
	c, _ := parseColor(opt.value)
	return &c
}

func (o *options) has(long string) bool { return o.get(long).seen }

// parse reads the command line, in the same order of checks as
// Options::parseCommandLine.
func (o *options) parse(args []string) error {
	if err := o.tokenize(args); err != nil {
		return err
	}
	ints := []struct {
		long string
		def  int
		p    *int
	}{
		{"pixels-per-second", 100, &o.pixelsPerSecond},
		{"bits", 16, &o.bits},
		{"width", 800, &o.width},
		{"height", 250, &o.height},
		{"bar-width", 8, &o.barWidth},
		{"bar-gap", 4, &o.barGap},
		{"compression", -1, &o.compression},
		{"raw-samplerate", 0, &o.rawSampleRate},
		{"raw-channels", 0, &o.rawChannels},
	}
	for _, v := range ints {
		*v.p = o.int(v.long, v.def)
	}
	o.start = o.float("start", 0)
	o.end = o.float("end", 0)
	o.border = o.color("border-color")
	o.background = o.color("background-color")
	o.axisLabel = o.color("axis-label-color")

	o.help = o.has("help")
	o.version = o.has("version")
	if o.help || o.version {
		return nil
	}
	o.quiet = o.has("quiet")
	o.splitChannels = o.has("split-channels")
	o.axisLabels = !o.has("no-axis-labels")
	o.hasEnd = o.has("end")
	o.hasSamplesPerPixel = o.has("zoom")
	o.hasPixelsPerSecond = o.has("pixels-per-second")
	o.hasBits = o.has("bits")
	o.colorScheme = o.str("colors", "audacity")
	o.waveformStyle = o.str("waveform-style", "normal")
	o.barStyle = o.str("bar-style", "square")
	o.rawFormat = o.str("raw-format", "")
	o.hasRawSampleRate = o.has("raw-samplerate")
	o.hasRawChanls = o.has("raw-channels")
	o.hasRawFormat = o.has("raw-format")

	if o.has("waveform-color") {
		for value := range strings.SplitSeq(strings.TrimSuffix(o.str("waveform-color", ""), ","), ",") {
			c, ok := parseColor(value)
			if !ok {
				return optionError{"Invalid waveform-color"}
			}
			o.waveformColors = append(o.waveformColors, c)
		}
	}

	o.input = o.str("input-filename", "")
	o.output = o.str("output-filename", "")
	o.hasInputFormat = o.has("input-format")
	o.hasOutputFormat = o.has("output-format")

	if o.input == "" && !o.hasInputFormat {
		return optionError{"Must specify either input filename or input format"}
	}
	o.inputFormat = formatFromExtension(o.input)
	if o.hasInputFormat {
		o.inputFormat = parseFormat(o.str("input-format", ""))
	}
	if err := o.parseAmplitudeScale(o.str("amplitude-scale", "1.0")); err != nil {
		return err
	}
	if err := o.parseZoom(o.str("zoom", "256")); err != nil {
		return err
	}
	if o.output == "" && !o.hasOutputFormat {
		return optionError{"Must specify either output filename or output format"}
	}
	o.outputFormat = formatFromExtension(o.output)
	if o.hasOutputFormat {
		o.outputFormat = parseFormat(o.str("output-format", ""))
	}
	if o.bits != 8 && o.bits != 16 {
		return optionError{"Invalid bits: must be either 8 or 16"}
	}
	if o.compression < -1 || o.compression > 9 {
		return optionError{"Invalid compression level: must be from 0 (none) to 9 (best), or -1 (default)"}
	}
	if o.inputFormat == formatRaw {
		switch {
		case !o.hasRawSampleRate:
			return optionError{"Missing --raw-samplerate option"}
		case !o.hasRawChanls:
			return optionError{"Missing --raw-channels option"}
		case !o.hasRawFormat:
			return optionError{"Missing --raw-format option"}
		}
	}
	return nil
}

var numberPattern = regexp.MustCompile(`^[+-]?[0-9]*(\.[0-9]*)?$`)

func (o *options) parseAmplitudeScale(s string) error {
	if s == "auto" {
		o.autoAmplitudeScale = true
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if !numberPattern.MatchString(s) || err != nil {
		return optionError{"Invalid amplitude scale: must be a number"}
	}
	if f < 0 {
		return optionError{"Invalid amplitude scale: must be a positive number"}
	}
	o.amplitudeScale = f
	return nil
}

// parseZoom accepts "auto" or what std::stoi accepts: leading spaces, a
// sign and digits, ignoring anything after them.
func (o *options) parseZoom(s string) error {
	if s == "auto" {
		o.autoSamplesPerPixel = true
		return nil
	}
	t := strings.TrimLeftFunc(s, unicode.IsSpace)
	n := 0
	if n < len(t) && (t[n] == '+' || t[n] == '-') {
		n++
	}
	digits := n
	for n < len(t) && t[n] >= '0' && t[n] <= '9' {
		n++
	}
	if n == digits {
		return optionError{"Invalid zoom: must be a number or 'auto'"}
	}
	v, err := strconv.ParseInt(t[:n], 10, 64)
	if err != nil || v > math.MaxInt32 || v < math.MinInt32 {
		return optionError{"Invalid zoom: number too large"}
	}
	o.samplesPerPixel = int(v)
	return nil
}

var colorPattern = regexp.MustCompile(`^[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)

func parseColor(s string) (color.NRGBA, bool) {
	if !colorPattern.MatchString(s) {
		return color.NRGBA{}, false
	}
	v, _ := strconv.ParseUint(s, 16, 32)
	if len(s) == 6 {
		v = v<<8 | 0xff
	}
	return color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, true
}

const helpText = `
Usage:
  %s [options]

Options:
  --help                          show help message
  -v [ --version ]                show version information
  -q [ --quiet ]                  disable progress and information messages
  -i [ --input-filename ] arg     input file name (.mp3, .wav, .flac, .ogg,
                                  .oga, .opus, .dat, .json)
  -o [ --output-filename ] arg    output file name (.wav, .dat, .png, .json)
  --split-channels                output multi-channel waveform data or image
                                  files
  --input-format arg              input file format (mp3, wav, flac, ogg, raw,
                                  opus, dat, json)
  --output-format arg             output file format (wav, dat, png, json)
  -z [ --zoom ] arg (=256)        zoom level (samples per pixel)
  --pixels-per-second arg (=100)  zoom level (pixels per second)
  -b [ --bits ] arg (=16)         bits (8 or 16)
  -s [ --start ] arg (=0)         start time (seconds)
  -e [ --end ] arg (=0)           end time (seconds)
  -w [ --width ] arg (=800)       image width (pixels)
  -h [ --height ] arg (=250)      image height (pixels)
  -c [ --colors ] arg (=audacity) color scheme (audition or audacity)
  --border-color arg              border color (rrggbb[aa])
  --background-color arg          background color (rrggbb[aa])
  --waveform-color arg            waveform color (rrggbb[aa])
  --waveform-style arg (=normal)  waveform style (normal or bars)
  --bar-width arg (=8)            bar width (pixels)
  --bar-gap arg (=4)              bar gap (pixels)
  --bar-style arg (=square)       bar style (square or rounded)
  --axis-label-color arg          axis label color (rrggbb[aa])
  --no-axis-labels                render waveform image without axis labels
  --with-axis-labels              render waveform image with axis labels
                                  (default)
  --amplitude-scale arg (=1.0)    amplitude scale
  --compression arg (=-1)         PNG compression level: 0 (none) to 9 (best),
                                  or -1 (default)
  --raw-samplerate arg            sample rate for raw audio input (Hz)
  --raw-channels arg              number of channels for raw audio input
  --raw-format arg                format for raw audio input (s8, u8, s16le,
                                  s16be, s24le, s24be, s32le, s32be, f32le,
                                  f32be, f64le, f64be)

See audiowaveform(1) for usage examples
`
