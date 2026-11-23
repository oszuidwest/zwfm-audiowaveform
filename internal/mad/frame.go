package mad

// Layers.
const (
	LayerI   = 1
	LayerII  = 2
	LayerIII = 3
)

// Channel modes.
const (
	ModeSingleChannel = 0
	ModeDualChannel   = 1
	ModeJointStereo   = 2
	ModeStereo        = 3
)

// Header flags.
const (
	FlagNPrivateIII = 0x0007
	FlagIncomplete  = 0x0008
	FlagProtection  = 0x0010
	FlagCopyright   = 0x0020
	FlagOriginal    = 0x0040
	FlagPadding     = 0x0080
	FlagIStereo     = 0x0100
	FlagMSStereo    = 0x0200
	FlagFreeFormat  = 0x0400
	FlagLSFExt      = 0x1000
	FlagMCExt       = 0x2000
	FlagMPEG25Ext   = 0x4000
)

const (
	privateHeader = 0x0100
)

// Header is libmad's mad_header (without the duration timer).
type Header struct {
	Layer         int
	Mode          int
	ModeExtension int
	Emphasis      int
	Bitrate       uint64
	SampleRate    uint32
	crcCheck      uint16
	crcTarget     uint16
	Flags         int
	PrivateBits   int
}

// Channels is MAD_NCHANNELS.
func (h *Header) Channels() int {
	if h.Mode != 0 {
		return 2
	}
	return 1
}

// nsbsamples is MAD_NSBSAMPLES.
func (h *Header) nsbsamples() int {
	switch {
	case h.Layer == LayerI:
		return 12
	case h.Layer == LayerIII && h.Flags&FlagLSFExt != 0:
		return 18
	}
	return 36
}

// Frame is libmad's mad_frame.
type Frame struct {
	Header   Header
	sbsample [2][36][32]fixed
	overlap  *[2][32][18]fixed
}

var bitrateTable = [5][15]uint64{
	// MPEG-1
	{0, 32000, 64000, 96000, 128000, 160000, 192000, 224000,
		256000, 288000, 320000, 352000, 384000, 416000, 448000},
	{0, 32000, 48000, 56000, 64000, 80000, 96000, 112000,
		128000, 160000, 192000, 224000, 256000, 320000, 384000},
	{0, 32000, 40000, 48000, 56000, 64000, 80000, 96000,
		112000, 128000, 160000, 192000, 224000, 256000, 320000},
	// MPEG-2 LSF
	{0, 32000, 48000, 56000, 64000, 80000, 96000, 112000,
		128000, 144000, 160000, 176000, 192000, 224000, 256000},
	{0, 8000, 16000, 24000, 32000, 40000, 48000, 56000,
		64000, 80000, 96000, 112000, 128000, 144000, 160000},
}

var samplerateTable = [3]uint32{44100, 48000, 32000}

// decodeHeader is decode_header.
func decodeHeader(h *Header, s *Stream) bool {
	h.Flags = 0
	h.PrivateBits = 0

	bufend := s.bitPtrAt(s.bufEnd)

	if bitLength(&s.ptr, &bufend) < 32 {
		s.Err = ErrBufLen
		return false
	}

	s.ptr.skip(11)

	if s.ptr.Read(1) == 0 {
		h.Flags |= FlagMPEG25Ext
	}

	if s.ptr.Read(1) == 0 {
		h.Flags |= FlagLSFExt
	} else if h.Flags&FlagMPEG25Ext != 0 {
		s.Err = ErrLostSync
		return false
	}

	h.Layer = 4 - int(s.ptr.Read(2))
	if h.Layer == 4 {
		s.Err = ErrBadLayer
		return false
	}

	if s.ptr.Read(1) == 0 {
		h.Flags |= FlagProtection
		h.crcCheck = crc(s.ptr, 16, 0xffff)
	}

	index := s.ptr.Read(4)
	if index == 15 {
		s.Err = ErrBadBitrate
		return false
	}
	if h.Flags&FlagLSFExt != 0 {
		h.Bitrate = bitrateTable[3+(h.Layer>>1)][index]
	} else {
		h.Bitrate = bitrateTable[h.Layer-1][index]
	}

	index = s.ptr.Read(2)
	if index == 3 {
		s.Err = ErrBadSampleRate
		return false
	}
	h.SampleRate = samplerateTable[index]
	if h.Flags&FlagLSFExt != 0 {
		h.SampleRate /= 2
		if h.Flags&FlagMPEG25Ext != 0 {
			h.SampleRate /= 2
		}
	}

	if s.ptr.Read(1) != 0 {
		h.Flags |= FlagPadding
	}
	if s.ptr.Read(1) != 0 {
		h.PrivateBits |= privateHeader
	}

	h.Mode = 3 - int(s.ptr.Read(2))
	h.ModeExtension = int(s.ptr.Read(2))

	if s.ptr.Read(1) != 0 {
		h.Flags |= FlagCopyright
	}
	if s.ptr.Read(1) != 0 {
		h.Flags |= FlagOriginal
	}

	h.Emphasis = int(s.ptr.Read(2))

	if h.Flags&FlagProtection != 0 {
		if bitLength(&s.ptr, &bufend) < 16 {
			s.Err = ErrBufLen
			return false
		}
		h.crcTarget = uint16(s.ptr.Read(16))
	}
	return true
}

func slotsPerFrame(h *Header) uint64 {
	if h.Layer == LayerIII && h.Flags&FlagLSFExt != 0 {
		return 72
	}
	return 144
}

// freeBitrate is free_bitrate.
func freeBitrate(s *Stream, h *Header) bool {
	keep := s.ptr
	var rate uint64

	var padSlot uint32
	if h.Flags&FlagPadding != 0 {
		padSlot = 1
	}
	spf := slotsPerFrame(h)

	for s.syncStream() {
		savedPtr, savedErr := s.ptr, s.Err
		peek := *h
		ok := decodeHeader(&peek, s)
		s.ptr, s.Err = savedPtr, savedErr

		if ok && peek.Layer == h.Layer && peek.SampleRate == h.SampleRate {
			n := uint32(s.ptr.nextByte() - s.ThisFrame)
			if h.Layer == LayerI {
				rate = uint64(h.SampleRate) * uint64(n-4*padSlot+4) / 48 / 1000
			} else {
				rate = uint64(h.SampleRate) * uint64(n-padSlot+1) / spf / 1000
			}
			if rate >= 8 {
				break
			}
		}
		s.ptr.skip(8)
	}

	s.ptr = keep

	if rate < 8 || (h.Layer == LayerIII && rate > 640) {
		s.Err = ErrLostSync
		return false
	}
	s.freerate = rate * 1000
	return true
}

func isSync(d []byte, p int) bool {
	return d[p] == 0xff && d[p+1]&0xe0 == 0xe0
}

// headerDecode is mad_header_decode.
func headerDecode(h *Header, s *Stream) bool {
	if !s.HasBuffer() {
		s.Err = ErrBufPtr
		s.sync = false
		return false
	}
	ptr := s.NextFrame
	end := s.bufEnd
	d := s.data

	for {
		if s.sync {
			if end-ptr < BufferGuard {
				s.NextFrame = ptr
				s.Err = ErrBufLen
				s.sync = false
				return false
			} else if end-ptr >= 2 && !isSync(d, ptr) {
				s.ThisFrame = ptr
				s.NextFrame = ptr + 1
				s.Err = ErrLostSync
				s.sync = false
				return false
			}
		} else {
			s.ptr = newBitPtr(d, ptr)
			if !s.syncStream() {
				if end-s.NextFrame >= BufferGuard {
					s.NextFrame = end - BufferGuard
				}
				s.Err = ErrBufLen
				s.sync = false
				return false
			}
			ptr = s.ptr.nextByte()
		}

		s.Err = ErrNone

		s.ThisFrame = ptr
		s.NextFrame = ptr + 1
		s.ptr = newBitPtr(d, s.ThisFrame)

		if !decodeHeader(h, s) {
			s.sync = false
			return false
		}

		if h.Bitrate == 0 {
			if (s.freerate == 0 || !s.sync ||
				(h.Layer == LayerIII && s.freerate > 640000)) &&
				!freeBitrate(s, h) {
				s.sync = false
				return false
			}
			h.Bitrate = s.freerate
			h.Flags |= FlagFreeFormat
		}

		var padSlot uint64
		if h.Flags&FlagPadding != 0 {
			padSlot = 1
		}
		var n uint32
		if h.Layer == LayerI {
			n = uint32((12*h.Bitrate/uint64(h.SampleRate) + padSlot) * 4)
		} else {
			n = uint32(slotsPerFrame(h)*h.Bitrate/uint64(h.SampleRate) + padSlot)
		}

		if int64(n)+BufferGuard > int64(end-s.ThisFrame) {
			s.NextFrame = s.ThisFrame
			s.Err = ErrBufLen
			s.sync = false
			return false
		}

		s.NextFrame = s.ThisFrame + int(n)

		if !s.sync {
			ptr = s.NextFrame
			if end-ptr >= 2 && !isSync(d, ptr) {
				ptr = s.ThisFrame + 1
				s.NextFrame = ptr
				continue
			}
			s.sync = true
		}
		break
	}

	h.Flags |= FlagIncomplete
	return true
}

// Decode is mad_frame_decode: it decodes the next frame of s into f.
// On failure it returns false and s.Err tells why.
func (f *Frame) Decode(s *Stream) bool {
	if f.Header.Flags&FlagIncomplete == 0 && !headerDecode(&f.Header, s) {
		return false
	}

	f.Header.Flags &^= FlagIncomplete

	var ok bool
	switch f.Header.Layer {
	case LayerI:
		ok = layerI(s, f)
	case LayerII:
		ok = layerII(s, f)
	default:
		ok = layerIII(s, f)
	}
	if !ok {
		if !s.Err.Recoverable() {
			s.NextFrame = s.ThisFrame
		}
		return false
	}

	if f.Header.Layer != LayerIII {
		s.AncPtr = s.ptr
	}
	return true
}
