package mp3dir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type memFile struct {
	io.ReadSeeker
	name    string
	size    int
	modtime time.Time
}

func (f *memFile) Readdir(int) ([]os.FileInfo, error) { return nil, errNotImplemented }
func (f *memFile) Stat() (os.FileInfo, error)         { return f, nil }
func (f *memFile) Close() error                       { return nil }
func (f *memFile) Name() string                       { return f.name }
func (f *memFile) Size() int64                        { return int64(f.size) }
func (f *memFile) Mode() os.FileMode                  { return 0444 }
func (f *memFile) ModTime() time.Time                 { return f.modtime }
func (f *memFile) IsDir() bool                        { return false }
func (f *memFile) Sys() interface{}                   { return nil }

func (md *MP3Dir) Open(name string) (http.File, error) {
	if name == "/index.json" {
		md.Lock()
		defer md.Unlock()
		intervals := make([][]int64, 0, len(md.onDisk))
		for _, seg := range md.onDisk {
			dur := seg.size * 8 / int64(md.BitRate)
			intervals = append(intervals, []int64{seg.unixts - dur, dur})
		}
		buf := &bytes.Buffer{}
		err := json.NewEncoder(buf).Encode(struct {
			Intervals [][]int64 `json:"intervals"`
		}{intervals})
		return &memFile{
			ReadSeeker: bytes.NewReader(buf.Bytes()),
			name:       "index.json",
			size:       buf.Len(),
			modtime:    time.Now(),
		}, err
	}
	var startts, endts int64
	_, err := fmt.Sscanf(name, "/%d-%d.mp3", &startts, &endts)
	if err != nil || endts < startts {
		return nil, os.ErrNotExist
	}
	return md.ReaderAt(time.Unix(startts, 0), time.Duration(endts-startts)*time.Second)
}
