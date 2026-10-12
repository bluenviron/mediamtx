package conf

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
)

func TestPathClone(t *testing.T) {
	original := &Path{
		Name:                "example",
		RTSPTransport:       RTSPTransport{new(gortsplib.ProtocolUDP)},
		SourceAnyPortEnable: new(true),
		RecordPath:          "/var/recordings",
	}

	clone := original.Clone()
	require.Equal(t, original, clone)
}

func TestIsValidPathName(t *testing.T) {
	for _, ca := range []struct {
		name   string
		path   string
		errMsg string
	}{
		{
			name: "valid nested path",
			path: "group/cam1",
		},
		{
			name: "valid dots inside segment",
			path: "cam.v1/main",
		},
		{
			name:   "parent directory",
			path:   "../cam1",
			errMsg: "can't contain dot path segments",
		},
		{
			name:   "embedded parent directory",
			path:   "group/../cam1",
			errMsg: "can't contain dot path segments",
		},
		{
			name:   "current directory",
			path:   "./cam1",
			errMsg: "can't contain dot path segments",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			err := IsValidPathName(ca.path)
			if ca.errMsg != "" {
				require.EqualError(t, err, ca.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRecordPathTimeZoneWarning(t *testing.T) {
	for _, ca := range []struct {
		name       string
		record     string
		recordPath string
		warn       bool
	}{
		{
			"without %z",
			"yes",
			"./recordings/%path/%Y-%m-%d_%H-%M-%S-%f",
			true,
		},
		{
			"with %z",
			"yes",
			"./recordings/%path/%Y-%m-%d_%H-%M-%S-%f_%z",
			false,
		},
		{
			"unix epoch",
			"yes",
			"./recordings/%path/%s.%f",
			false,
		},
		{
			"recording disabled",
			"no",
			"./recordings/%path/%Y-%m-%d_%H-%M-%S-%f",
			false,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			tmpf := createTempFile(t, []byte(
				"paths:\n"+
					"  test:\n"+
					"    record: "+ca.record+"\n"+
					"    recordPath: "+ca.recordPath+"\n"))

			l := &warningCounter{}
			_, _, err := Load(tmpf, nil, l)
			require.NoError(t, err)

			if ca.warn {
				require.Equal(t, 1, l.count)
				require.Contains(t, l.last, "path 'test'")
				require.Contains(t, l.last, "%z")
			} else {
				require.Equal(t, 0, l.count)
			}
		})
	}
}

type warningCounter struct {
	count int
	last  string
}

// Log implements logger.Writer.
func (w *warningCounter) Log(level logger.Level, format string, args ...any) {
	if level == logger.Warn {
		w.count++
		w.last = fmt.Sprintf(format, args...)
	}
}
