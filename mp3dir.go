package mp3dir

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/tcolgate/mp3"
)

type segment struct {
	filename string
	unixts   int64
	size     int64
}

type MP3Dir struct {
	Root    string
	BitRate int

	loaded     bool
	onDisk     []segment // includes current.mp3
	onDiskSize int64     // includes current.mp3

	nextRefresh time.Time
	sync.Mutex
}

func (md *MP3Dir) ReaderAt(start time.Time, max time.Duration) (http.File, error) {
	md.Lock()
	defer md.Unlock()
	md.refreshDirState()

	var startts, endts int64
	startts = start.Unix()
	if max == 0 {
		endts = time.Now().Unix()
	} else {
		endts = start.Add(max).Unix()
	}
	var want []segment
	for _, seg := range md.onDisk {
		if seg.unixts <= startts || seg.size == 0 {
			continue
		} else if seg.unixts < endts {
			want = append(want, seg)
		} else {
			want = append(want, seg)
			break
		}
	}
	if len(want) == 0 {
		return nil, os.ErrNotExist
	}
	skip := want[0].size - int64((time.Unix(want[0].unixts, 0).Sub(start)).Seconds()*float64(md.BitRate)/8)
	if skip < 0 {
		skip = 0
	}
	want[len(want)-1].size -= (want[len(want)-1].unixts - endts) * int64(md.BitRate) / 8

	skip, err := nextFrameStart(filepath.Join(md.Root, want[0].filename), skip)
	if err != nil {
		return nil, err
	}

	return &reader{
		md:       md,
		name:     fmt.Sprintf("%d-%d.mp3", startts, endts),
		modtime:  time.Unix(want[len(want)-1].unixts, 0),
		segments: want,
		skip:     skip,
	}, nil
}

// caller must have lock.
func (md *MP3Dir) refreshDirState() error {
	now := time.Now()
	if !now.After(md.nextRefresh) {
		return nil
	}
	onDisk, onDiskSize, err := md.loadDirState()
	if err != nil {
		return err
	}
	md.onDisk, md.onDiskSize = onDisk, onDiskSize
	md.nextRefresh = now.Add(5 * time.Second)
	return nil
}

func (md *MP3Dir) loadDirState() (onDisk []segment, onDiskSize int64, err error) {
	dir, err := os.Open(md.Root)
	if err != nil {
		return
	}
	fis, err := dir.Readdir(0)
	if err != nil {
		return
	}
	for _, fi := range fis {
		var unixts int64
		if _, err := fmt.Sscanf(fi.Name(), finishedFilenameFormat, &unixts); err == nil {
			// unixts is end time
		} else if fi.Name() == currentFilename {
			unixts = fi.ModTime().Unix()
		} else {
			continue
		}
		onDisk = append(onDisk, segment{filename: fi.Name(), unixts: unixts, size: fi.Size()})
		onDiskSize += fi.Size()
	}
	sort.Slice(onDisk, func(i, j int) bool {
		return onDisk[j].filename == currentFilename || onDisk[i].unixts < onDisk[j].unixts
	})
	return
}

func nextFrameStart(filename string, pos int64) (int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	_, err = f.Seek(pos, io.SeekStart)
	if err != nil {
		return 0, err
	}
	dec := mp3.NewDecoder(f)
	var frame mp3.Frame
	var skipSync int
	err = dec.Decode(&frame, &skipSync)
	switch err {
	case nil:
		return pos + int64(skipSync), nil
	case io.EOF:
		return f.Seek(0, io.SeekCurrent)
	default:
		return 0, err
	}
}
