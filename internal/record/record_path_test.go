package record

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var recordPathCases = []struct {
	name   string
	format string
	dec    *recordPathParams
	enc    string
}{
	{
		"standard",
		"%path/%Y-%m-%d_%H-%M-%S-%f.mp4",
		&recordPathParams{
			path: "mypath",
			time: time.Date(2008, 11, 0o7, 11, 22, 4, 123456000, time.Local),
		},
		"mypath/2008-11-07_11-22-04-123456.mp4",
	},
	{
		"unix seconds",
		"%path/%s.mp4",
		&recordPathParams{
			path: "mypath",
			time: time.Date(2021, 12, 2, 12, 15, 23, 0, time.UTC).Local(),
		},
		"mypath/1638447323.mp4",
	},
}

func TestRecordPathDecode(t *testing.T) {
	for _, ca := range recordPathCases {
		t.Run(ca.name, func(t *testing.T) {
			require.Equal(t, ca.dec, decodeRecordPath(ca.format, ca.enc))
		})
	}
}

func TestRecordPathEncode(t *testing.T) {
	for _, ca := range recordPathCases {
		t.Run(ca.name, func(t *testing.T) {
			require.Equal(t, ca.enc, strings.ReplaceAll(encodeRecordPath(ca.dec, ca.format), "%path", "mypath"))
		})
	}
}
