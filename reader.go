package mp3dir

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type readCloser struct {
	io.Reader
	io.Closer
}

type reader struct {
	root     string
	segments []segment
	current  io.ReadCloser
	err      error
}

// Read from the current/next segment.
func (r *reader) Read(p []byte) (int, error) {
	n := 0
	for n == 0 && r.err == nil {
		if r.current == nil {
			if len(r.segments) == 0 {
				r.err = io.EOF
				break
			}
			seg := r.segments[0]
			r.segments = r.segments[1:]
			f, err := os.Open(filepath.Join(r.root, fmt.Sprintf(finishedFilenameFormat, seg.unixts)))
			if err != nil {
				r.err = err
				break
			}
			r.current = &readCloser{
				Reader: &io.LimitedReader{R: f, N: seg.size},
				Closer: r.current,
			}
		}
		n, r.err = r.current.Read(p)
		if r.err == io.EOF {
			r.current.Close()
			r.current = nil
			r.err = nil
		}
	}
	return n, r.err
}

// Close any open files.
func (r *reader) Close() error {
	if r.current == nil {
		return nil
	}
	err := r.current.Close()
	r.current = nil
	return err
}
