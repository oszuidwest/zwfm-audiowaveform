package main

import (
	"path/filepath"
	"strings"
)

type format int

const (
	formatUnknown format = iota
	formatMP3
	formatWAV
	formatFLAC
	formatOgg
	formatOpus
	formatRaw
	formatDat
	formatJSON
	formatTxt
	formatPNG
)

var formatNames = map[string]format{
	"mp3": formatMP3, "wav": formatWAV, "w64": formatWAV, "flac": formatFLAC,
	"ogg": formatOgg, "oga": formatOgg, "opus": formatOpus, "raw": formatRaw,
	"dat": formatDat, "json": formatJSON, "txt": formatTxt, "png": formatPNG,
}

func parseFormat(name string) format {
	return formatNames[strings.ToLower(name)]
}

func formatFromExtension(filename string) format {
	return parseFormat(strings.TrimPrefix(filepath.Ext(filename), "."))
}

func (f format) String() string {
	return [...]string{"", "mp3", "wav", "flac", "ogg", "opus", "raw", "dat", "json", "txt", "png"}[f]
}

func (f format) isAudio() bool { return f >= formatMP3 && f <= formatRaw }

func (f format) isWaveformData() bool { return f == formatDat || f == formatJSON }
