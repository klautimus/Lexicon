package library

import (
	"io"
	"os"

	"github.com/dhowden/tag"
)

const maxTagReadSize = 10 * 1024 * 1024 // 10 MB limit for embedded artwork/ID3 tags

type readSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

// openReader opens the file at path with a size limit to prevent DoS via
// large embedded artwork. Files larger than maxTagReadSize are still opened
// but reads are limited to the first maxTagReadSize bytes (sufficient for
// ID3/APEv2 tags which are typically at the start/end of the file).
func openReader(path string) (readSeekCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if stat.Size() > maxTagReadSize {
		// Wrap in a limited reader to cap how much data tag.ReadFrom can consume.
		// ID3v1 tags are at most 128 bytes at end-of-file, ID3v2 at start.
		// 10 MB is more than enough for any valid tag.
		return &limitedReadSeekCloser{
			rs: f,
			N:  maxTagReadSize,
		}, nil
	}
	return f, nil
}

type limitedReadSeekCloser struct {
	rs              readSeekCloser
	N               int64
}

func (l *limitedReadSeekCloser) Read(p []byte) (int, error) {
	return io.LimitReader(l.rs, l.N).Read(p)
}

func (l *limitedReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	return l.rs.Seek(offset, whence)
}

func (l *limitedReadSeekCloser) Close() error {
	return l.rs.Close()
}

func readCover(f io.ReadSeeker) *tag.Picture {
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil
	}
	return m.Picture()
}
