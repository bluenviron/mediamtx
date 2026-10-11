package logger_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
)

func TestLoggerToStdout(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"structured",
	} {
		t.Run(ca, func(t *testing.T) {
			var buf bytes.Buffer

			l := &logger.Logger{
				Destinations: []logger.Destination{logger.DestinationStdout},
				Structured:   (ca == "structured"),
				TimeNow:      func() time.Time { return time.Date(2003, 11, 4, 23, 15, 8, 431232, time.UTC) },
				Stdout:       &buf,
			}
			err := l.Initialize()
			require.NoError(t, err)
			defer l.Close()

			l.Log(logger.Info, "test format %d", 123)

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
			tempFile, err := os.CreateTemp(t.TempDir(), "mtx-logger-")
			require.NoError(t, err)
			defer tempFile.Close()

			l := &logger.Logger{
				Level:        logger.Debug,
				Destinations: []logger.Destination{logger.DestinationFile},
				Structured:   ca == "structured",
				File:         tempFile.Name(),
				TimeNow:      func() time.Time { return time.Date(2003, 11, 4, 23, 15, 8, 0, time.UTC) },
			}
			err = l.Initialize()
			require.NoError(t, err)
			defer l.Close()

			l.Log(logger.Info, "test format %d", 123)

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

func TestLoggerReload(t *testing.T) {
	var buf bytes.Buffer

	l := &logger.Logger{
		Level:        logger.Warn,
		Destinations: []logger.Destination{logger.DestinationStdout},
		TimeNow:      func() time.Time { return time.Date(2003, 11, 4, 23, 15, 8, 431232, time.UTC) },
		Stdout:       &buf,
	}
	err := l.Initialize()
	require.NoError(t, err)
	defer l.Close()

	l.Log(logger.Info, "ignored")
	require.Equal(t, "", buf.String())

	l.Level = logger.Info
	l.Structured = true
	err = l.Reload()
	require.NoError(t, err)

	l.Log(logger.Info, "test")
	require.Equal(t, `{"timestamp":"2003-11-04T23:15:08.000431232Z","level":"INF","message":"test"}`+"\n", buf.String())

	// failure keeps the previous configuration
	buf.Reset()
	l.Level = logger.Debug
	l.Destinations = []logger.Destination{logger.DestinationFile}
	l.Structured = false
	l.File = "/nonexistent/dir/file.log"
	err = l.Reload()
	require.Error(t, err)

	l.Log(logger.Debug, "ignored")
	require.Equal(t, "", buf.String())
	l.Log(logger.Info, "test")
	require.NotEqual(t, "", buf.String())
}
