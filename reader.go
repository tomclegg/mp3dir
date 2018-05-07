package mp3dir

import (
	"errors"
	"io"
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
	md *MP3Dir

	name     string
	modtime  time.Time
	segments []segment
	skip     int64

	seek    int64
	segoff  int64
	started bool
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
	if !r.started {
		r.started = true
		r.segoff = r.skip + r.seek
	}
	n := 0
	for n == 0 && r.err == nil {
		if r.current == nil {
			if len(r.segments) == 0 {
				r.err = io.EOF
				break
			}
			seg := r.segments[0]
			r.segments = r.segments[1:]
			if r.segoff >= seg.size {
				r.segoff -= seg.size
				continue
			}
			f, err := os.Open(filepath.Join(r.md.Root, seg.filename))

			// Check whether the "current" file was
			// renamed since we decided to read it. If so,
			// ignore any error opening current, and open
			// the renamed file instead.
			if seg.filename != currentFilename {
				// no race
			} else if onDisk, _, errLoad := r.md.loadDirState(); errLoad != nil {
				err = errLoad
			} else {
				// The file we want is the oldest file
				// whose endtime is >= the endtime the
				// "current" file had when we decided
				// to read it.
				for _, s := range onDisk {
					if s.unixts < seg.unixts {
						continue
					}
					if s.filename == seg.filename {
						// no race
						break
					}
					// current.mp3 was renamed
					if f != nil {
						f.Close()
					}
					f, err = os.Open(filepath.Join(r.md.Root, s.filename))
					break
				}
			}

			if err != nil {
				r.err = err
				break
			}
			if r.segoff > 0 {
				_, err = f.Seek(r.segoff, io.SeekStart)
				if err != nil {
					f.Close()
					r.err = err
					break
				}
			}
			r.current = &readCloser{
				Reader: &io.LimitedReader{R: f, N: seg.size - r.segoff},
				Closer: f,
			}
			r.segoff = 0
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
