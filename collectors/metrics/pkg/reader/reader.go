package reader

import (
	"fmt"
	"io"
)

type limitReadCloser struct {
	io.Reader
	closer io.ReadCloser
}

func NewLimitReadCloser(r io.ReadCloser, n int64) io.ReadCloser {
	return limitReadCloser{
		Reader: LimitReader(r, n),
		closer: r,
	}
}

func (c limitReadCloser) Close() error {
	return c.closer.Close()
}

var ErrTooLong = fmt.Errorf("the incoming sample data is too long")

// LimitReader returns a Reader that reads from r
// but stops with ErrTooLong after n bytes.
// The underlying implementation is a *LimitedReader.
func LimitReader(r io.Reader, n int64) io.Reader { return &LimitedReader{r, n} }

// A LimitedReader reads from R but limits the amount of
// data returned to just N bytes. Each call to Read
// updates N to reflect the new amount remaining.
// Read returns ErrTooLong when N <= 0 or when the underlying R returns EOF.
type LimitedReader struct {
	R io.Reader // underlying reader
	N int64     // max bytes remaining
}

func (l *LimitedReader) Read(p []byte) (n int, err error) {
	if l.N <= 0 {
		return 0, ErrTooLong
	}
	if int64(len(p)) > l.N {
		p = p[0:l.N]
	}
	n, err = l.R.Read(p)
	l.N -= int64(n)
	return
}
