package sndfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// oggPage holds the packets that complete on one Ogg page of the first
// logical stream, with that page's granule position.
type oggPage struct {
	granule int64
	eos     bool
	packets [][]byte
}

// oggDemux splits the first logical stream of an Ogg file into packets.
type oggDemux struct {
	r          io.Reader
	serial     uint32
	haveSerial bool
	partial    []byte
}

// next returns the next page on which at least one packet completes, or
// io.EOF at the end of the stream.
func (o *oggDemux) next() (*oggPage, error) {
	for {
		var hdr [27]byte
		if _, err := io.ReadFull(o.r, hdr[:]); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				err = io.EOF
			}
			return nil, err
		}
		if string(hdr[:4]) != "OggS" || hdr[4] != 0 {
			return nil, errors.New("Error in Ogg file: lost page sync")
		}
		lacing := make([]byte, hdr[26])
		if _, err := io.ReadFull(o.r, lacing); err != nil {
			return nil, io.EOF
		}
		bodyLen := 0
		for _, l := range lacing {
			bodyLen += int(l)
		}
		body := make([]byte, bodyLen)
		if _, err := io.ReadFull(o.r, body); err != nil {
			return nil, io.EOF
		}
		serial := binary.LittleEndian.Uint32(hdr[14:])
		if !o.haveSerial {
			o.serial, o.haveSerial = serial, true
		}
		if serial != o.serial {
			continue
		}
		if hdr[5]&0x01 == 0 {
			o.partial = o.partial[:0] // not a continuation: drop any partial packet
		}
		page := &oggPage{
			granule: int64(binary.LittleEndian.Uint64(hdr[6:])),
			eos:     hdr[5]&0x04 != 0,
		}
		off := 0
		for _, l := range lacing {
			o.partial = append(o.partial, body[off:off+int(l)]...)
			off += int(l)
			if l < 255 {
				page.packets = append(page.packets, bytes.Clone(o.partial))
				o.partial = o.partial[:0]
			}
		}
		if len(page.packets) > 0 || page.eos {
			return page, nil
		}
	}
}

// oggCRCTable is the lookup table for the Ogg page checksum (polynomial
// 0x04c11db7, no reflection).
var oggCRCTable = func() (t [256]uint32) {
	for i := range t {
		r := uint32(i) << 24
		for range 8 {
			if r&0x80000000 != 0 {
				r = r<<1 ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		t[i] = r
	}
	return t
}()

// oggCRC returns the checksum of a complete page, with the checksum field
// counted as zero.
func oggCRC(page []byte) uint32 {
	var crc uint32
	for i, b := range page {
		if i >= 22 && i < 26 {
			b = 0
		}
		crc = crc<<8 ^ oggCRCTable[byte(crc>>24)^b]
	}
	return crc
}

// lastGranule finds the granule position of the last valid page of the given
// stream: the page libsndfile's ogg_sync_last_page_before finds. The tail
// read starts at 128 KiB and doubles until it holds a page of the stream or
// the whole input.
func lastGranule(src *source, serial uint32) (int64, bool) {
	for n := int64(1 << 17); ; n *= 2 {
		tail, err := src.readTail(n)
		if err != nil && len(tail) == 0 {
			return 0, false
		}
		gp, ok := int64(0), false
		for i := 0; i+27 <= len(tail); i++ {
			if string(tail[i:i+4]) != "OggS" || tail[i+4] != 0 {
				continue
			}
			nseg := int(tail[i+26])
			if i+27+nseg > len(tail) {
				continue
			}
			size := 27 + nseg
			for _, l := range tail[i+27 : i+27+nseg] {
				size += int(l)
			}
			if i+size > len(tail) {
				continue
			}
			p := tail[i : i+size]
			if oggCRC(p) != binary.LittleEndian.Uint32(p[22:]) {
				continue
			}
			g := int64(binary.LittleEndian.Uint64(p[6:]))
			if binary.LittleEndian.Uint32(p[14:]) == serial && g != -1 {
				gp, ok = g, true
			}
			i += size - 1
		}
		if ok || int64(len(tail)) < n {
			return gp, ok
		}
	}
}

// openOgg reads the first page and dispatches on the codec of its first
// packet.
func openOgg(src *source) (*reader, error) {
	demux := &oggDemux{r: src}
	first, err := demux.next()
	if err != nil || len(first.packets) == 0 {
		return nil, errors.New("Error in Ogg file: no packets")
	}
	pkt := first.packets[0]
	switch {
	case len(pkt) >= 7 && pkt[0] == 1 && string(pkt[1:7]) == "vorbis":
		return openVorbis(src, demux, first)
	case len(pkt) >= 8 && string(pkt[:8]) == "OpusHead":
		return openOpus(src, demux, first)
	}
	return nil, errors.New("File contains data in an unimplemented format.")
}
