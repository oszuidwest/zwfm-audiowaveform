package sndfile

import (
	"errors"
	"io"
)

// source wraps the input with peeking and, when the input can seek, the
// stream length and random access to its tail.
type source struct {
	r      io.Reader
	buf    []byte // peeked bytes not yet consumed
	pos    int64  // bytes consumed so far
	seeker io.Seeker
	start  int64 // offset of the stream start in the seeker
	end    int64 // stream length in the seeker, -1 if unknown
}

func newSource(r io.Reader) *source {
	s := &source{r: r, end: -1}
	sk, ok := r.(io.Seeker)
	if !ok {
		return s
	}
	cur, err := sk.Seek(0, io.SeekCurrent)
	if err != nil {
		return s // a pipe
	}
	end, err := sk.Seek(0, io.SeekEnd)
	if err != nil {
		return s
	}
	if _, err := sk.Seek(cur, io.SeekStart); err != nil {
		return s
	}
	s.seeker, s.start, s.end = sk, cur, end
	return s
}

// peek returns up to n bytes without consuming them.
func (s *source) peek(n int) ([]byte, error) {
	if len(s.buf) < n {
		more := make([]byte, n-len(s.buf))
		m, err := io.ReadFull(s.r, more)
		s.buf = append(s.buf, more[:m]...)
		if err != nil {
			return s.buf, err
		}
	}
	return s.buf[:n], nil
}

func (s *source) Read(p []byte) (int, error) {
	if len(s.buf) > 0 {
		n := copy(p, s.buf)
		s.buf = s.buf[n:]
		s.pos += int64(n)
		return n, nil
	}
	n, err := s.r.Read(p)
	s.pos += int64(n)
	return n, err
}

// skip discards n bytes. It fails with io.ErrUnexpectedEOF if the stream
// ends first.
func (s *source) skip(n int64) error {
	if n <= 0 {
		return nil
	}
	m, err := io.CopyN(io.Discard, s, n)
	if m < n && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return err
}

// size reports the number of bytes left from the current position, if the
// input can seek.
func (s *source) size() (int64, bool) {
	if s.seeker == nil {
		return 0, false
	}
	return s.end - s.start - s.pos, true
}

// readTail returns the last n bytes of the stream, if the input can seek.
// The read position is restored afterwards.
func (s *source) readTail(n int64) ([]byte, error) {
	if s.seeker == nil {
		return nil, errors.New("not seekable")
	}
	cur, err := s.seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	n = min(n, s.end-s.start)
	if _, err := s.seeker.Seek(s.end-n, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	_, rerr := io.ReadFull(s.r, buf)
	if _, err := s.seeker.Seek(cur, io.SeekStart); err != nil {
		return nil, err
	}
	return buf, rerr
}
