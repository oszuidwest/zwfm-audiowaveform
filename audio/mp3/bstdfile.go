package mp3

import (
	"bufio"
	"errors"
	"io"
)

// bstdFile mirrors the bstdfile helper of madlld that audiowaveform reads
// MP3 input through. Its end-of-file flag is set as soon as a read has
// delivered the last byte of the input, which decides when the decoder gets
// its MAD_BUFFER_GUARD padding.
type bstdFile struct {
	r   *bufio.Reader
	eof bool
	err error
}

func newBstdFile(r io.Reader) *bstdFile {
	return &bstdFile{r: bufio.NewReaderSize(r, 8192)}
}

// read fills p as fread would and returns the number of bytes read.
func (b *bstdFile) read(p []byte) int {
	if len(p) == 0 || b.eof || b.err != nil {
		return 0
	}

	n, err := io.ReadFull(b.r, p)
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		b.eof = true
		return n
	case err != nil:
		b.err = err
		return n
	}

	// bstdfile reads ahead after every request; an empty read ahead marks
	// the end of the file.
	if _, err := b.r.Peek(1); err != nil {
		if errors.Is(err, io.EOF) {
			b.eof = true
		} else {
			b.err = err
		}
	}
	return n
}
