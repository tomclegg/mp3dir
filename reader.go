package mp3dir

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

type readCloser struct {
	io.Reader
	io.Closer
}

// reader implements http.File and os.FileInfo
type reader struct {
	root     string
	name     string
	modtime  time.Time
	segments []segment
	skip     int64

	seek    int64
	current io.ReadCloser
	err     error
}

var errNotImplemented = errors.New("not implemented")

func (r *reader) Readdir(int) ([]os.FileInfo, error) {
	return nil, errNotImplemented
}

func (r *reader) Stat() (os.FileInfo, error) {
	return r, nil
}

// Seek implements io.Seeker, but only if called before the first
// read. After Read has been called, results are undefined.
func (r *reader) Seek(offset int64, whence int) (int64, error) {
	if r.current != nil {
		r.err = errNotImplemented
	} else if whence == io.SeekStart {
		r.seek = offset
	} else if whence == io.SeekCurrent {
		r.seek += offset
	} else if whence == io.SeekEnd {
		r.seek = r.Size() + offset
	}
	return r.seek, nil
}

// Read from the current/next segment.
func (r *reader) Read(p []byte) (int, error) {
	r.seek += r.skip
	n := 0
	for n == 0 && r.err == nil {
		if r.current == nil {
			log.Printf("r.segments==%#v", r.segments)
			if len(r.segments) == 0 {
				r.err = io.EOF
				break
			}
			seg := r.segments[0]
			r.segments = r.segments[1:]
			if r.seek >= seg.size {
				r.seek -= seg.size
				continue
			}
			f, err := os.Open(filepath.Join(r.root, fmt.Sprintf(finishedFilenameFormat, seg.unixts)))
			if err != nil {
				r.err = err
				break
			}
			if r.seek > 0 {
				_, err = f.Seek(r.seek, io.SeekStart)
				if err != nil {
					f.Close()
					r.err = err
					break
				}
				r.seek = 0
			}
			r.current = &readCloser{
				Reader: &io.LimitedReader{R: f, N: seg.size},
				Closer: f,
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

func (r *reader) Name() string {
	return r.name
}

func (r *reader) Size() int64 {
	var size int64
	for _, seg := range r.segments {
		size += seg.size
	}
	size -= r.skip
	return size
}

func (r *reader) Mode() os.FileMode {
	return 0444
}

func (r *reader) ModTime() time.Time {
	return r.modtime
}

func (r *reader) IsDir() bool {
	return false
}

func (r *reader) Sys() interface{} {
	return nil
}
