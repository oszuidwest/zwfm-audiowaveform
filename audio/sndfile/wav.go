package sndfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	waveFormatPCM        = 0x0001
	waveFormatIEEEFloat  = 0x0003
	waveFormatExtensible = 0xfffe
)

// guidTail is the last 14 bytes of every KSDATAFORMAT_SUBTYPE_* GUID; the
// first two bytes hold the format code.
var guidTail = []byte{0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}

type wavFormat struct {
	channels   int
	sampleRate int
	enc        pcmEncoding
}

// parseFmt decodes a 'fmt ' chunk body like wavlike_read_fmt_chunk.
func parseFmt(b []byte, order binary.ByteOrder, bigEndian bool) (wavFormat, error) {
	if len(b) < 16 {
		return wavFormat{}, errors.New("Error in WAV file. No 'fmt ' chunk marker.")
	}
	format := order.Uint16(b[0:])
	f := wavFormat{
		channels:   int(order.Uint16(b[2:])),
		sampleRate: int(order.Uint32(b[4:])),
	}
	bits := int(order.Uint16(b[14:]))
	if format == waveFormatExtensible {
		if len(b) < 40 {
			return wavFormat{}, errors.New("Error in WAV file. Bad 'fmt ' chunk.")
		}
		sub := order.Uint16(b[24:])
		if !bytes.Equal(b[26:40], guidTail) {
			return wavFormat{}, errors.New("Unsupported encoding in WAV file.")
		}
		format = sub
	}
	width := (bits + 7) / 8
	switch format {
	case waveFormatPCM:
		switch width {
		case 1:
			f.enc = pcmEncoding{kind: pcmU8, width: 1}
		case 2, 3, 4:
			f.enc = pcmEncoding{kind: pcmInt, width: width, bigEndian: bigEndian}
		default:
			return wavFormat{}, fmt.Errorf("Unsupported bit width %d in WAV file.", bits)
		}
	case waveFormatIEEEFloat:
		if width == 8 {
			f.enc = pcmEncoding{kind: pcmFloat, width: 8, bigEndian: bigEndian}
		} else {
			f.enc = pcmEncoding{kind: pcmFloat, width: 4, bigEndian: bigEndian}
		}
	default:
		return wavFormat{}, fmt.Errorf("Unsupported WAV format 0x%04x.", format)
	}
	if f.channels < 1 {
		return wavFormat{}, errZeroChannels
	}
	if f.channels > 1024 {
		return wavFormat{}, errors.New("Error : too many channels in file.")
	}
	return f, nil
}

// maxChunkRead is how many bytes of a 'fmt ' or 'ds64' body the parsers use.
// The rest is skipped, so a bogus chunk size cannot force a huge allocation.
const maxChunkRead = 40

// readChunk reads the first maxChunkRead bytes of a chunk body of size bytes
// and skips the rest.
func readChunk(src *source, size int64) ([]byte, error) {
	body := make([]byte, min(size, maxChunkRead))
	if _, err := io.ReadFull(src, body); err != nil {
		return nil, err
	}
	return body, src.skip(size - int64(len(body)))
}

// readFmt reads and parses a 'fmt ' chunk body. It returns bad if the body
// is cut short.
func readFmt(src *source, size int64, order binary.ByteOrder, bigEndian bool, bad error) (wavFormat, error) {
	body, err := readChunk(src, size)
	if err != nil {
		return wavFormat{}, bad
	}
	return parseFmt(body, order, bigEndian)
}

// openWAV parses a RIFF, RIFX, RF64 or BW64 header up to the 'data' chunk.
func openWAV(src *source) (*reader, error) {
	var hdr [12]byte
	if _, err := io.ReadFull(src, hdr[:]); err != nil {
		return nil, errors.New("Error in WAV file. No 'RIFF' chunk marker.")
	}
	var order binary.ByteOrder = binary.LittleEndian
	bigEndian := string(hdr[:4]) == "RIFX"
	if bigEndian {
		order = binary.BigEndian
	}
	rf64 := string(hdr[:4]) == "RF64" || string(hdr[:4]) == "BW64"
	riffSize := order.Uint32(hdr[4:])

	var (
		ds64Data int64 = -1
		haveFmt  bool
		f        wavFormat
		chunk    [8]byte
	)
	for {
		if _, err := io.ReadFull(src, chunk[:]); err != nil {
			if !haveFmt {
				return nil, errors.New("Error in WAV file. No 'fmt ' chunk marker.")
			}
			return nil, errors.New("Error in WAV file. No 'data' chunk marker.")
		}
		id := string(chunk[:4])
		size := int64(order.Uint32(chunk[4:]))
		switch id {
		case "ds64":
			if !rf64 {
				break
			}
			body, err := readChunk(src, size)
			if err != nil || size < 16 {
				return nil, errors.New("Error in RF64 file. Bad 'ds64' chunk.")
			}
			ds64Data = int64(binary.LittleEndian.Uint64(body[8:]))
			if err := src.skip(size & 1); err != nil {
				return nil, err
			}
			continue
		case "fmt ":
			if haveFmt {
				break // a second 'fmt ' chunk is ignored
			}
			var err error
			if f, err = readFmt(src, size, order, bigEndian, errors.New("Error in WAV file. Bad 'fmt ' chunk.")); err != nil {
				return nil, err
			}
			haveFmt = true
			if err := src.skip(size & 1); err != nil {
				return nil, err
			}
			continue
		case "data":
			if !haveFmt {
				return nil, errors.New("Error in WAV file. No 'fmt ' chunk marker.")
			}
			if rf64 && size == 0xffffffff && ds64Data >= 0 {
				size = ds64Data
			}
			// libsndfile treats an unfinished header (RIFF size 8, data
			// size 0) and a 0xffffffff size as running to the end of file.
			unknown := (size == 0 && riffSize == 8) || size == 0xffffffff
			return newWAVReader(src, f, size, unknown), nil
		}
		if err := src.skip(size + size&1); err != nil {
			return nil, errors.New("Error in WAV file. No 'data' chunk marker.")
		}
	}
}

// newWAVReader builds the reader once the data chunk is found. size is the
// data chunk size; when the input can seek it is clamped to the bytes left in
// the file, as wav_read_header and pcm_init do.
func newWAVReader(src *source, f wavFormat, size int64, unknown bool) *reader {
	block := int64(f.enc.width * f.channels)
	dataBytes := size
	if left, ok := src.size(); ok {
		if unknown || dataBytes > left {
			dataBytes = left
		}
	} else if unknown {
		dataBytes = -1
	}
	var frames int64
	if dataBytes >= 0 {
		frames = dataBytes / block
		dataBytes = frames * block
	}
	return &reader{
		sampleRate: f.sampleRate,
		channels:   f.channels,
		frames:     frames,
		dec:        newPCMDecoder(src, f.enc, f.channels, dataBytes),
	}
}

var (
	w64FmtGUID  = []byte{'f', 'm', 't', ' ', 0xf3, 0xac, 0xd3, 0x11, 0x8c, 0xd1, 0x00, 0xc0, 0x4f, 0x8e, 0xdb, 0x8a}
	w64DataGUID = []byte{'d', 'a', 't', 'a', 0xf3, 0xac, 0xd3, 0x11, 0x8c, 0xd1, 0x00, 0xc0, 0x4f, 0x8e, 0xdb, 0x8a}
)

// openW64 parses a Sony Wave64 header. Chunk sizes include the 24-byte chunk
// header and chunks are aligned to 8 bytes.
func openW64(src *source) (*reader, error) {
	var hdr [40]byte // riff GUID, size, wave GUID
	if _, err := io.ReadFull(src, hdr[:]); err != nil {
		return nil, errors.New("Error in W64 file. Bad header.")
	}
	var (
		haveFmt bool
		f       wavFormat
		chunk   [24]byte
	)
	for {
		if _, err := io.ReadFull(src, chunk[:]); err != nil {
			return nil, errors.New("Error in W64 file. No 'data' chunk.")
		}
		size := int64(binary.LittleEndian.Uint64(chunk[16:])) - 24
		if size < 0 {
			return nil, errors.New("Error in W64 file. Bad chunk size.")
		}
		pad := (8 - size%8) % 8
		switch {
		case bytes.Equal(chunk[:16], w64FmtGUID) && !haveFmt:
			var err error
			if f, err = readFmt(src, size, binary.LittleEndian, false, errors.New("Error in W64 file. Bad 'fmt ' chunk.")); err != nil {
				return nil, err
			}
			haveFmt = true
			if err := src.skip(pad); err != nil {
				return nil, err
			}
			continue
		case bytes.Equal(chunk[:16], w64DataGUID):
			if !haveFmt {
				return nil, errors.New("Error in W64 file. No 'fmt ' chunk.")
			}
			return newWAVReader(src, f, size, false), nil
		}
		if err := src.skip(size + pad); err != nil {
			return nil, errors.New("Error in W64 file. No 'data' chunk.")
		}
	}
}
