package mp3dir

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	currentFilename        = "current.mp3"
	finishedFilenameFormat = "t%d.mp3"
)

// Writer implements io.WriteCloser by appending to an mp3dir.
type Writer struct {
	MP3Dir
	SplitOnSize    int64
	SplitOnSilence time.Duration
	PurgeOnSize    int64
	OnCloseError   func(error)

	loaded      bool
	current     io.WriteCloser
	currentSize int64
	lastWrite   time.Time
	err         error
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	w.MP3Dir.Lock()
	defer w.MP3Dir.Unlock()
	if !w.loaded {
		// TODO: acquire a lockfile to ensure no other process
		// is changing w.Root underneath us.
		w.timestampCurrent()
		err := w.refreshDirState()
		if err != nil {
			return 0, err
		}
		w.loaded = true
	}
	w.open(len(p))
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.current.Write(p)
	w.currentSize += int64(n)
	w.lastWrite = time.Now()
	return n, err
}

// Close implements io.Closer
func (w *Writer) Close() error {
	w.closeCurrent()
	return w.err
}

// open a new file if necessary. If an error occurs, w.err will be
// non-nil. Otherwise, w.current will be non-nil and suitable for
// writing size bytes.
func (w *Writer) open(size int) {
	switch {
	case w.current == nil:
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
	err := w.timestampCurrent()
	if w.err == nil {
		w.err = err
	}
	if w.err != nil {
		return
	}
	w.current, w.err = os.OpenFile(filepath.Join(w.Root, currentFilename), os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0777)
}

func (w *Writer) closeCurrent() {
	if w.current == nil {
		return
	}
	if w.err = w.current.Close(); w.err != nil {
		if w.OnCloseError == nil {
			log.Printf("error closing segment: %s (writer %v)", w.err, w.current)
		} else {
			w.OnCloseError(w.err)
		}
	}
	w.current = nil
	w.currentSize = 0
}

// rename current.mp3 to t%d.mp3 and purge
func (w *Writer) timestampCurrent() error {
	oldname := filepath.Join(w.Root, currentFilename)
	fi, err := os.Stat(oldname)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	unixts := fi.ModTime().Unix()
	err = os.Rename(oldname, filepath.Join(w.Root, fmt.Sprintf(finishedFilenameFormat, unixts)))
	if err != nil {
		return err
	}
	w.onDisk = append(w.onDisk, segment{unixts: unixts, size: fi.Size()})
	w.onDiskSize += fi.Size()
	return w.purge()
}

func (w *Writer) purge() error {
	if w.PurgeOnSize <= 0 {
		return nil
	}
	var err error
	purged := 0
	for len(w.onDisk) > purged && w.onDiskSize > w.PurgeOnSize {
		err = os.Remove(filepath.Join(w.Root, fmt.Sprintf(finishedFilenameFormat, w.onDisk[purged].unixts)))
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
