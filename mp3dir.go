package mp3dir

import (
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"sync"
	"time"
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

	refresh   *time.Ticker
	setupOnce sync.Once
	sync.Mutex
}

func (md *MP3Dir) NewReaderAt(start time.Time, max time.Duration) (io.ReadCloser, error) {
	md.refreshDirState()
	md.Lock()
	defer md.Unlock()

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
		if seg.unixts <= startts {
			continue
		} else if seg.unixts < endts {
			want = append(want, seg)
		} else {
			// want part of this segment
			seg.size -= (seg.unixts - endts) * int64(md.BitRate) / 8
			if seg.size > 0 {
				want = append(want, seg)
			}
			done = true
			break
		}
	}
	if !done {
		// TODO: consider current.mp3
		log.Printf("TODO: consider current.mp3")
	}
	return &reader{root: md.Root, segments: want}, nil
}

func (md *MP3Dir) refreshDirState() error {
	var err error
	md.setupOnce.Do(func() {
		md.refresh = time.NewTicker(5 * time.Second)
		err = md.loadDirState()
	})
	select {
	case <-md.refresh.C:
		err = md.loadDirState()
	default:
	}
	return err
}

func (md *MP3Dir) loadDirState() error {
	dir, err := os.Open(md.Root)
	if err != nil {
		return err
	}
	fis, err := dir.Readdir(0)
	if err != nil {
		return err
	}
	md.onDisk = nil
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
