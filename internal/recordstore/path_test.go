package recordstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var pathCases = []struct {
	name   string
	format string
	dec    Path
	enc    string
}{
	{
		"standard",
		"%path/%Y-%m-%d_%H-%M-%S-%f.mp4",
		Path{
			Start: time.Date(2008, 11, 0o7, 11, 22, 4, 123456000, time.Local),
			Path:  "mypath",
		},
		"mypath/2008-11-07_11-22-04-123456.mp4",
	},
	{
		"unix seconds",
		"%path/%s.mp4",
		Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 0, time.UTC).Local(),
			Path:  "mypath",
		},
		"mypath/1638447323.mp4",
	},
}

func TestPathDecode(t *testing.T) {
	for _, ca := range pathCases {
		t.Run(ca.name, func(t *testing.T) {
			var dec Path
			ok := dec.Decode(ca.format, ca.enc)
			require.Equal(t, true, ok)
			require.Equal(t, ca.dec, dec)
		})
	}
}

func TestPathEncode(t *testing.T) {
	for _, ca := range pathCases {
		t.Run(ca.name, func(t *testing.T) {
			require.Equal(t, ca.enc, ca.dec.Encode(ca.format))
		})
	}
}
