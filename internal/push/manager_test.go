package push

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type testPathManager struct{}

func (*testPathManager) AddReader(defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
	return nil, fmt.Errorf("no stream is available")
}

type testLogger struct{}

func (*testLogger) Log(logger.Level, string, ...any) {}

func TestManager(t *testing.T) {
	m := &Manager{
		PathName:    "test",
		PathManager: &testPathManager{},
		Parent:      &testLogger{},
	}
	m.Initialize(conf.PushTargets{{URL: "rtmp://localhost/app/stream"}})
	defer m.Close()

	list := m.List()
	require.Len(t, list.Items, 1)
	require.Equal(t, defs.APIPushTargetSourceConfig, list.Items[0].Source)

	_, err := m.Add("rtmp://localhost/app/stream")
	require.ErrorIs(t, err, ErrTargetAlreadyExists)

	added, err := m.Add("rtsp://localhost:8554/stream")
	require.NoError(t, err)
	require.Equal(t, defs.APIPushTargetSourceAPI, added.APIItem().Source)

	m.ReloadConf(conf.PushTargets{{URL: "rtsp://localhost:8554/stream"}})
	list = m.List()
	require.Len(t, list.Items, 1)
	require.Equal(t, "rtsp://localhost:8554/stream", list.Items[0].URL)
	require.Equal(t, defs.APIPushTargetSourceConfig, list.Items[0].Source)

	_, err = m.Get(uuid.New())
	require.ErrorIs(t, err, ErrTargetNotFound)

	err = m.Remove(list.Items[0].ID)
	require.NoError(t, err)

	m.ReloadConf(conf.PushTargets{{URL: "srt://localhost:8890?streamid=publish:test"}})
	list = m.List()
	require.Len(t, list.Items, 1)
	require.Equal(t, "srt://localhost:8890?streamid=publish:test", list.Items[0].URL)
}

func TestRTMPFourCCList(t *testing.T) {
	require.Empty(t, rtmpFourCCList(&description.Session{Medias: []*description.Media{{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp: 96,
		}},
	}}}))

	require.NotEmpty(t, rtmpFourCCList(&description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.H265{}},
	}}}))
}

func TestRTMPURLWithDefaultPort(t *testing.T) {
	for _, ca := range []struct {
		rawURL string
		host   string
	}{
		{"rtmp://example.com/live/stream", "example.com:1935"},
		{"rtmps://example.com/live/stream", "example.com:1936"},
		{"rtmp://example.com:1937/live/stream", "example.com:1937"},
	} {
		t.Run(ca.rawURL, func(t *testing.T) {
			u, err := url.Parse(ca.rawURL)
			require.NoError(t, err)
			require.Equal(t, ca.host, rtmpURLWithDefaultPort(u).Host)
		})
	}
}
