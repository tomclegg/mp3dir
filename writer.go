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

	loaded    bool
	current   io.WriteCloser
	lastWrite time.Time
	err       error
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
	now := time.Now()
	w.onDisk[len(w.onDisk)-1].unixts = now.Unix()
	w.onDisk[len(w.onDisk)-1].size += int64(n)
	w.onDiskSize += int64(n)
	w.lastWrite = now

	// Don't bother refreshing state from disk if we're doing the
	// writing ourselves
	w.MP3Dir.nextRefresh = w.lastWrite.Add(5 * time.Second)

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
	case w.SplitOnSize > 0 && w.onDisk[len(w.onDisk)-1].size+int64(size) > w.SplitOnSize:
		// this write would exceed SplitOnSize
	case w.SplitOnSilence > 0 && time.Since(w.lastWrite) > w.SplitOnSilence:
		// reopen after a silent period
	default:
		// don't need to reopen
		return
	}
	w.err = nil
	w.closeCurrent()
	err := w.timestampCurrent()
	if w.err == nil {
		w.err = err
	}
	if w.err != nil {
		return
	}
	w.current, w.err = os.OpenFile(filepath.Join(w.Root, currentFilename), os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0777)
	if w.err == nil {
		w.onDisk = append(w.onDisk, segment{filename: currentFilename, unixts: time.Now().Unix()})
	}
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
}

// rename current.mp3 to t%d.mp3 and purge
func (w *Writer) timestampCurrent() error {
	if len(w.onDisk) == 0 || w.onDisk[len(w.onDisk)-1].filename != currentFilename {
		return nil
	}
	unixts := w.onDisk[len(w.onDisk)-1].unixts
	newname := fmt.Sprintf(finishedFilenameFormat, unixts)
	err := os.Rename(filepath.Join(w.Root, currentFilename), filepath.Join(w.Root, newname))
	if err != nil {
		return err
	}
	w.onDisk[len(w.onDisk)-1].filename = newname
	w.onDisk[len(w.onDisk)-1].unixts = unixts
	return w.purge()
}

func (w *Writer) purge() error {
	if w.PurgeOnSize <= 0 {
		return nil
	}
	var err error
	purged := 0
	for len(w.onDisk)-1 > purged && w.onDiskSize+w.SplitOnSize > w.PurgeOnSize {
		target := filepath.Join(w.Root, w.onDisk[purged].filename)
		err = os.Remove(target)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("error removing %s: %s", target, err)
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
