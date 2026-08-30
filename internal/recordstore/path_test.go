package recordstore_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

var pathCases = []struct {
	name   string
	format string
	dec    recordstore.Path
	enc    string
}{
	{
		"standard",
		"%path/%Y-%m-%d_%H-%M-%S-%f.mp4",
		recordstore.Path{
			Start: time.Date(2008, 11, 7, 11, 22, 4, 123456000, time.Local),
			Path:  "mypath",
		},
		"mypath/2008-11-07_11-22-04-123456.mp4",
	},
	{
		"unix seconds",
		"%path/%s.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 0, time.UTC).Local(),
			Path:  "mypath",
		},
		"mypath/1638447323.mp4",
	},
	{
		"unix microseconds",
		"%path/%s.%f.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 567324000, time.UTC).Local(),
			Path:  "mypath",
		},
		"mypath/1638447323.567324.mp4",
	},
	{
		"timezone utc",
		"%path/%Y-%m-%d_%H-%M-%S-%f_%z.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 567324000, time.UTC),
			Path:  "mypath",
		},
		"mypath/2021-12-02_12-15-23-567324_Z.mp4",
	},
	{
		"timezone plus",
		"%path/%Y-%m-%d_%H-%M-%S-%f_%z.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 567324000, time.FixedZone("myzone", 7200)),
			Path:  "mypath",
		},
		"mypath/2021-12-02_12-15-23-567324_+0200.mp4",
	},
	{
		"timezone minus",
		"%path/%Y-%m-%d_%H-%M-%S-%f_%z.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 567324000, time.FixedZone("myzone", -7200)),
			Path:  "mypath",
		},
		"mypath/2021-12-02_12-15-23-567324_-0200.mp4",
	},
	{
		"timezone plus fractional hour",
		"%path/%Y-%m-%d_%H-%M-%S-%f_%z.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 567324000, time.FixedZone("myzone", 19800)),
			Path:  "mypath",
		},
		"mypath/2021-12-02_12-15-23-567324_+0530.mp4",
	},
	{
		"timezone plus fractional hour 45",
		"%path/%Y-%m-%d_%H-%M-%S-%f_%z.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 567324000, time.FixedZone("myzone", 20700)),
			Path:  "mypath",
		},
		"mypath/2021-12-02_12-15-23-567324_+0545.mp4",
	},
	{
		"timezone minus fractional hour",
		"%path/%Y-%m-%d_%H-%M-%S-%f_%z.mp4",
		recordstore.Path{
			Start: time.Date(2021, 12, 2, 12, 15, 23, 567324000, time.FixedZone("myzone", -12600)),
			Path:  "mypath",
		},
		"mypath/2021-12-02_12-15-23-567324_-0330.mp4",
	},
}

func TestPathDecode(t *testing.T) {
	for _, ca := range pathCases {
		t.Run(ca.name, func(t *testing.T) {
			var dec recordstore.Path
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

// berry's, CSV timestamp sidecar
var csvPathCases = []struct {
	name    string
	segment string
	csv     string
	ok      bool
}{
	{
		"fmp4",
		"/rec/mypath/2008-11-07_11-22-04-123456.mp4",
		"/rec/mypath/2008-11-07_11-22-04-123456.csv",
		true,
	},
	{
		"mpegts has no sidecar",
		"/rec/mypath/2008-11-07_11-22-04-123456.ts",
		"",
		false,
	},
	{
		"extension only, not first occurrence in directory",
		"/rec.mp4archive/mypath/2008-11-07_11-22-04-123456.mp4",
		"/rec.mp4archive/mypath/2008-11-07_11-22-04-123456.csv",
		true,
	},
	{
		"extension only, not first occurrence in path name",
		"/rec/cam.mp4/2008-11-07_11-22-04-123456.mp4",
		"/rec/cam.mp4/2008-11-07_11-22-04-123456.csv",
		true,
	},
}

func TestCSVPath(t *testing.T) {
	for _, ca := range csvPathCases {
		t.Run(ca.name, func(t *testing.T) {
			csv, ok := recordstore.CSVPath(ca.segment)
			require.Equal(t, ca.ok, ok)
			require.Equal(t, ca.csv, csv)
		})
	}
}
