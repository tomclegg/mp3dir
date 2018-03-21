package mp3dir

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func (md *MP3Dir) Open(name string) (http.File, error) {
	var startts, endts int64
	_, err := fmt.Sscanf(name, "/%d-%d.mp3", &startts, &endts)
	if err != nil || endts < startts {
		return nil, os.ErrNotExist
	}
	return md.ReaderAt(time.Unix(startts, 0), time.Duration(endts-startts)*time.Second)
}
