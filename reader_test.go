package mp3dir

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	check "gopkg.in/check.v1"
)

type readerSuite struct{}

var _ = check.Suite(&readerSuite{})

var frameSilent64k = []byte{
	0xff, 0xfb, 0x54, 0xc4, 0x00, 0x03, 0xc0, 0x00, 0x01, 0xa4, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00,
	0x34, 0x80, 0x00, 0x00, 0x04, 0x4c, 0x41, 0x4c, 0x41, 0x4d, 0x45, 0x33, 0x2e, 0x39, 0x39, 0x2e,
	0x35, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
}

func (*readerSuite) TestReaderAtFrameBoundary(c *check.C) {
	tmpdir, err := ioutil.TempDir("", "mp3dir-test-")
	c.Assert(err, check.IsNil)
	defer os.RemoveAll(tmpdir)

	md := &MP3Dir{
		Root:    tmpdir,
		BitRate: 64000,
	}

	silenceStart, silenceDuration := time.Unix(1234567890, 0), 10*time.Second
	for j := 1; j <= 4; j++ {
		f, err := os.OpenFile(filepath.Join(tmpdir, fmt.Sprintf("t%d.mp3", silenceStart.Add(time.Duration(j)*silenceDuration).Unix())), os.O_CREATE|os.O_WRONLY, 0644)
		c.Assert(err, check.IsNil)
		defer f.Close()
		for i := int(float64(md.BitRate) * silenceDuration.Seconds() / 8); i >= 0; {
			n, err := f.Write(frameSilent64k)
			c.Assert(err, check.IsNil)
			i -= n
		}
		c.Assert(f.Close(), check.IsNil)
	}

	for offset := silenceDuration * 9 / 10; offset < silenceDuration*11/10; offset += 77 * time.Millisecond {
		c.Logf("offset %v", offset)
		r, err := md.ReaderAt(silenceStart.Add(offset), time.Second)
		defer r.Close()

		fi, err := r.Stat()
		c.Check(err, check.IsNil)
		size := fi.Size()

		buf, err := ioutil.ReadAll(r)
		c.Check(err, check.IsNil)
		c.Check(int64(len(buf)), check.Equals, size)
		c.Check(len(buf) > len(frameSilent64k), check.Equals, true)
		if len(buf) > 0 {
			c.Check(buf[0], check.Equals, frameSilent64k[0])
			c.Check(buf[1], check.Equals, frameSilent64k[1])
		}
		if len(buf) > len(frameSilent64k) {
			c.Check(buf[len(frameSilent64k)], check.Equals, frameSilent64k[0])
		}
	}
}
