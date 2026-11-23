package mad

// BufferGuard is MAD_BUFFER_GUARD: the number of zero bytes that must follow
// the last frame of a stream so it can be decoded.
const BufferGuard = 8

// mainDataLen is MAD_BUFFER_MDLEN.
const mainDataLen = 511 + 2048 + BufferGuard

// Error is libmad's mad_error.
type Error int

// Decoder errors, with the values of enum mad_error.
const (
	ErrNone           Error = 0x0000
	ErrBufLen         Error = 0x0001
	ErrBufPtr         Error = 0x0002
	ErrNoMem          Error = 0x0031
	ErrLostSync       Error = 0x0101
	ErrBadLayer       Error = 0x0102
	ErrBadBitrate     Error = 0x0103
	ErrBadSampleRate  Error = 0x0104
	ErrBadEmphasis    Error = 0x0105
	ErrBadCRC         Error = 0x0201
	ErrBadBitAlloc    Error = 0x0211
	ErrBadScalefactor Error = 0x0221
	ErrBadMode        Error = 0x0222
	ErrBadFrameLen    Error = 0x0231
	ErrBadBigValues   Error = 0x0232
	ErrBadBlockType   Error = 0x0233
	ErrBadScfsi       Error = 0x0234
	ErrBadDataPtr     Error = 0x0235
	ErrBadPart3Len    Error = 0x0236
	ErrBadHuffTable   Error = 0x0237
	ErrBadHuffData    Error = 0x0238
	ErrBadStereo      Error = 0x0239
)

// Recoverable is MAD_RECOVERABLE.
func (e Error) Recoverable() bool {
	return e&0xff00 != 0
}

func (e Error) Error() string {
	switch e {
	case ErrNone:
		return "no error"
	case ErrBufLen:
		return "input buffer too small (or EOF)"
	case ErrBufPtr:
		return "invalid (null) buffer pointer"
	case ErrNoMem:
		return "not enough memory"
	case ErrLostSync:
		return "lost synchronization"
	case ErrBadLayer:
		return "reserved header layer value"
	case ErrBadBitrate:
		return "forbidden bitrate value"
	case ErrBadSampleRate:
		return "reserved sample frequency value"
	case ErrBadEmphasis:
		return "reserved emphasis value"
	case ErrBadCRC:
		return "CRC check failed"
	case ErrBadBitAlloc:
		return "forbidden bit allocation value"
	case ErrBadScalefactor:
		return "bad scalefactor index"
	case ErrBadMode:
		return "bad bitrate/mode combination"
	case ErrBadFrameLen:
		return "bad frame length"
	case ErrBadBigValues:
		return "bad big_values count"
	case ErrBadBlockType:
		return "reserved block_type"
	case ErrBadScfsi:
		return "bad scalefactor selection info"
	case ErrBadDataPtr:
		return "bad main_data_begin pointer"
	case ErrBadPart3Len:
		return "bad audio data length"
	case ErrBadHuffTable:
		return "bad Huffman table select"
	case ErrBadHuffData:
		return "Huffman data overrun"
	case ErrBadStereo:
		return "incompatible block_type for JS"
	}
	return "unknown error"
}

// Stream is libmad's mad_stream. Frame positions are byte offsets into the
// buffer passed to SetBuffer.
type Stream struct {
	data   []byte // the caller's whole buffer
	bufEnd int    // end of the valid bytes in data

	sync     bool
	freerate uint64

	// ThisFrame and NextFrame are the offsets of the current and the next
	// frame (this_frame and next_frame).
	ThisFrame int
	NextFrame int

	ptr BitPtr

	// AncPtr points at the ancillary bits of the last decoded frame.
	AncPtr BitPtr

	mainData *[mainDataLen]byte
	mdLen    uint32
	l3       *layer3State

	// Err is the error of the last failed call.
	Err Error
}

// HasBuffer reports whether SetBuffer has been called.
func (s *Stream) HasBuffer() bool {
	return s.data != nil
}

// BufEnd returns the offset of the end of the buffer (bufend).
func (s *Stream) BufEnd() int {
	return s.bufEnd
}

// SetBuffer is mad_stream_buffer: the stream decodes buf[:length]. Bytes of
// buf after length may still be read by the bit reader, as in libmad.
func (s *Stream) SetBuffer(buf []byte, length int) {
	s.data = buf
	s.bufEnd = length
	s.ThisFrame = 0
	s.NextFrame = 0
	s.sync = true
	s.ptr = newBitPtr(buf, 0)
}

// syncStream is mad_stream_sync.
func (s *Stream) syncStream() bool {
	ptr := s.ptr.nextByte()
	end := s.bufEnd
	d := s.data
	for ptr < end-1 && !(d[ptr] == 0xff && d[ptr+1]&0xe0 == 0xe0) {
		ptr++
	}
	if end-ptr < BufferGuard {
		return false
	}
	s.ptr = newBitPtr(d, ptr)
	return true
}

func (s *Stream) bitPtrAt(pos int) BitPtr {
	return newBitPtr(s.data, pos)
}
