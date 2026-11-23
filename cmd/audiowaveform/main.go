// Command audiowaveform generates waveform data and renders waveform images
// from audio files. It is a Go port of audiowaveform by Chris Needham and
// BBC Research & Development, with the same options and output.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is the audiowaveform release this port matches.
const version = "1.11.1"

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin, stdout *os.File, stderr io.Writer) int {
	prog := args[0]
	o := newOptions()
	if err := o.parse(args[1:]); err != nil {
		fmt.Fprintf(stderr, "Error: %s\nSee '%s --help' for available options\n", err, prog)
		return 1
	}
	if o.help {
		fmt.Fprintf(stdout, "AudioWaveform v%s\n"+helpText, version, prog)
		return 0
	}
	if o.version {
		fmt.Fprintf(stdout, "AudioWaveform v%s\n", version)
		return 0
	}
	a := &app{o: o, info: stderr, stdin: stdin, stdout: stdout}
	if o.quiet {
		a.info = io.Discard
	}
	if err := a.run(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	a.infof("Done\n")
	return 0
}
