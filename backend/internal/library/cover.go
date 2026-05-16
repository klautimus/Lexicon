package library

import (
	"io"
	"os"

	"github.com/dhowden/tag"
)

type readSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

func openReader(path string) (readSeekCloser, error) {
	return os.Open(path)
}

func readCover(f io.ReadSeeker) *tag.Picture {
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil
	}
	return m.Picture()
}
