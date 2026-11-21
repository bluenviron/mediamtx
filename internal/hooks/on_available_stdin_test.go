package hooks

import (
	"fmt"
	"sync"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type recordingLogger struct {
	mutex sync.Mutex
	lines []string
}

func (l *recordingLogger) Log(_ logger.Level, format string, args ...any) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func TestOnAvailableStdinUnsupportedCodecs(t *testing.T) {
	strm := &stream.Stream{
		OrigDesc: &description.Session{Medias: []*description.Media{{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{PayloadTyp: 96}},
		}}},
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            &recordingLogger{},
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	pool := &externalcmd.Pool{}
	pool.Initialize()
	defer pool.Close()

	l := &recordingLogger{}

	stop := OnAvailableStdin(OnAvailableStdinParams{
		Logger:          l,
		ExternalCmdPool: pool,
		Conf: &conf.Path{
			RunOnAvailableStdin:        "sh -c 'cat > /dev/null'",
			RunOnAvailableStdinRestart: true,
		},
		ExternalCmdEnv: externalcmd.Environment{},
		Stream:         strm,
	})
	stop()

	l.mutex.Lock()
	defer l.mutex.Unlock()
	require.NotContains(t, l.lines, "runOnAvailableStdin command started")
	require.Contains(t, l.lines,
		"runOnAvailableStdin command not started: the stream doesn't contain any supported codec, which are currently "+
			"H265, H264, MPEG-4 Video, MPEG-1/2 Video, Opus, MPEG-4 Audio, MPEG-1/2 Audio, AC-3")
}
