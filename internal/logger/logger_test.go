package logger

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoggerToStdout(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"structured",
	} {
		t.Run(ca, func(t *testing.T) {
			var buf bytes.Buffer

			l := &Logger{
				Destinations: []Destination{DestinationStdout},
				Structured:   (ca == "structured"),
				timeNow:      func() time.Time { return time.Date(2003, 11, 4, 23, 15, 8, 431232, time.UTC) },
				stdout:       &buf,
			}
			err := l.Initialize()
			require.NoError(t, err)
			defer l.Close()

			l.Log(Info, "test format %d", 123)

			if ca == "plain" {
				require.Equal(t, "2003/11/04 23:15:08 INF test format 123\n", buf.String())
			} else {
				require.Equal(t, `{"timestamp":"2003-11-04T23:15:08.000431232Z",`+
					`"level":"INF","message":"test format 123"}`+"\n", buf.String())
			}
		})
	}
}

func TestLoggerToFile(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"structured",
	} {
		t.Run(ca, func(t *testing.T) {
			tempFile, err := os.CreateTemp(os.TempDir(), "mtx-logger-")
			require.NoError(t, err)
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			l := &Logger{
				Level:        Debug,
				Destinations: []Destination{DestinationFile},
				Structured:   ca == "structured",
				File:         tempFile.Name(),
				timeNow:      func() time.Time { return time.Date(2003, 11, 4, 23, 15, 8, 0, time.UTC) },
			}
			err = l.Initialize()
			require.NoError(t, err)
			defer l.Close()

			l.Log(Info, "test format %d", 123)

			buf, err := os.ReadFile(tempFile.Name())
			require.NoError(t, err)

			if ca == "plain" {
				require.Equal(t, "2003/11/04 23:15:08 INF test format 123\n", string(buf))
			} else {
				require.Equal(t, `{"timestamp":"2003-11-04T23:15:08Z",`+
					`"level":"INF","message":"test format 123"}`+"\n", string(buf))
			}
		})
	}
}
