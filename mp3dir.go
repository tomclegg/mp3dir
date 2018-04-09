package mp3dir

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/tcolgate/mp3"
)

type segment struct {
	unixts int64
	size   int64
}

type MP3Dir struct {
	Root    string
	BitRate int

	loaded     bool
	onDisk     []segment
	onDiskSize int64

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
	done := false
	for _, seg := range md.onDisk {
		if seg.unixts <= startts || seg.size == 0 {
			continue
		} else if seg.unixts < endts {
			want = append(want, seg)
		} else {
			want = append(want, seg)
			done = true
			break
		}
	}
	if !done {
		log.Printf("TODO: consider current.mp3")
	}
	if len(want) == 0 {
		return nil, os.ErrNotExist
	}
	skip := want[0].size - int64((time.Unix(want[0].unixts, 0).Sub(start)).Seconds()*float64(md.BitRate)/8)
	if skip < 0 {
		skip = 0
	}
	want[len(want)-1].size -= (want[len(want)-1].unixts - endts) * int64(md.BitRate) / 8

	skip, err := nextFrameStart(filepath.Join(md.Root, fmt.Sprintf(finishedFilenameFormat, want[0].unixts)), skip)
	if err != nil {
		return nil, err
	}

	return &reader{
		root:     md.Root,
		name:     fmt.Sprintf("%d-%d.mp3", startts, endts),
		modtime:  time.Unix(want[len(want)-1].unixts, 0),
		segments: want,
		skip:     skip,
	}, nil
}

// caller must have lock.
func (md *MP3Dir) refreshDirState() error {
	if now := time.Now(); now.After(md.nextRefresh) {
		err := md.loadDirState()
		if err != nil {
			return err
		}
		md.nextRefresh = now.Add(5 * time.Second)
	}
	return nil
}

// caller must have lock.
func (md *MP3Dir) loadDirState() error {
	dir, err := os.Open(md.Root)
	if err != nil {
		return err
	}
	fis, err := dir.Readdir(0)
	if err != nil {
		return err
	}
	md.onDisk, md.onDiskSize = nil, 0
	for _, fi := range fis {
		var unixts int64
		if _, err := fmt.Sscanf(fi.Name(), finishedFilenameFormat, &unixts); err != nil {
			continue
		}
		md.onDisk = append(md.onDisk, segment{unixts: unixts, size: fi.Size()})
		md.onDiskSize += fi.Size()
	}
	sort.Slice(md.onDisk, func(i, j int) bool {
		return md.onDisk[i].unixts < md.onDisk[j].unixts
	})
	return nil
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
