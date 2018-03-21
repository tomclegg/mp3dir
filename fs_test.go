package mp3dir

import (
	"net/http"
	"testing"

	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type Suite struct{}

var _ = check.Suite(&Suite{})

func (*Suite) TestInterface(c *check.C) {
	var _ http.FileSystem = &MP3Dir{}
}
