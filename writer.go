package mp3dir

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	currentFilename        = "current.mp3"
	finishedFilenameFormat = "t%d.mp3"
)

// Writer implements io.WriteCloser by writing files to an mp3dir.
type Writer struct {
	Dir            string
	SplitOnSize    int64
	SplitOnSilence time.Duration
	PurgeOnSize    int64
	OnCloseError   func(error)

	current     io.WriteCloser
	currentSize int64
	lastWrite   time.Time
	err         error

	loaded     bool
	onDisk     []segment
	onDiskSize int64
}

type segment struct {
	unixts int64
	size   int64
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	if !loaded {
		w.timestampCurrent()
		err := w.loadDirState()
		if err != nil {
			return 0, err
		}
		loaded = true
	}
	w.open(len(p))
	if w.err != nil {
		return 0, err
	}
	n, err := w.current.Write(p)
	w.currentSize += int64(n)
	w.lastWrite = time.Now()
	return n, err
}

// Close implements io.Closer
func (w *Writer) Close() error {
	w.closeCurrent()
}

// open a new file if necessary. If an error occurs, w.err will be
// non-nil. Otherwise, w.current will be non-nil and suitable for
// writing size bytes.
func (w *Writer) open(size int) {
	switch {
	case w.writer == nil:
		// never opened (or last open failed)
	case w.err != nil:
		// last open failed
	case w.SplitOnSize > 0 && w.currentSize+int64(size) > w.SplitOnSize:
		// this write would exceed SplitOnSize
	case w.SplitOnSilence > 0 && time.Since(w.lastWrite) > w.SplitOnSilence:
		// reopen after a silent period
	default:
		// don't need to reopen
		return
	}
	w.closeCurrent()
	w.err = w.timestampCurrent()
	if w.err != nil {
		return
	}
	w.writer, w.err = os.OpenFile(filepath.Join(w.Dir, currentFilename), os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0777)
}

func (w *Writer) closeCurrent() {
	if w.writer == nil {
		return
	}
	if err := w.current.Close(); err != nil {
		if w.OnCloseError == nil {
			log.Printf("error closing segment: %s (writer %v)", err, w.current)
		} else {
			w.OnCloseError(err)
		}
	}
	w.writer = nil
	w.currentSize = 0
}

func (w *Writer) loadDirState() {
	dir, err := os.Open(w.Dir)
	if err != nil {
		return err
	}
	fis, err := dir.Readdir(0)
	if err != nil {
		return err
	}
	w.onDisk = nil
	for _, fi := range fis {
		var unixts int
		if n, err := fmt.Sscanf(fi.Name(), finishedFilenameFormat, &unixts); err != nil {
			continue
		}
		w.onDisk = append(w.onDisk, segment{unixts: unixts, size: fi.Size()})
		w.onDiskSize += fi.Size()
	}
	sort.Slice(w.onDisk, func(i, j int) bool {
		return w.onDisk[i].unixts < w.onDisk[j].unixts
	})
}

// rename current.mp3 to t%d.mp3 and purge
func (w *Writer) timestampCurrent() error {
	oldname := filepath.Join(w.Dir, currentFilename)
	fi, err := os.Stat(oldname)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	unixts := fi.ModTime().Unix()
	err := os.Rename(oldname, filepath.Join(w.Dir, fmt.Sprintf(finishedFilenameFormat, unixts)))
	if err != nil {
		return err
	}
	w.onDisk = append(w.onDisk, segment{unixts: unixts, size: fi.Size()})
	return w.purge()
}

func (w *Writer) purge() error {
	var err error
	purged := 0
	for len(w.onDisk) > 0 && w.onDiskSize > w.PurgeOnSize {
		err = os.Remove(filepath.Join(w.Dir, fmt.Sprintf(finishedFilenameFormat, w.onDisk[purged].unixts)))
		if err != nil {
			break
		}
		w.onDiskSize -= w.onDisk[purged].size
		purged++
	}
	if purged > 0 {
		remain := make([]segment, len(w.onDisk)-purged, cap(w.onDisk))
		copy(remain, w.onDisk[purged:])
		w.onDisk = remain
	}
	return err
}
