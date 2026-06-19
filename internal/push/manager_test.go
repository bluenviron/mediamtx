package push

import (
	"fmt"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type testPathManager struct{}

func (*testPathManager) AddReader(defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
	return nil, fmt.Errorf("no stream is available")
}

type testLogger struct{}

func (*testLogger) Log(logger.Level, string, ...any) {}

type testBlockingPath struct {
	removeReaderStarted chan struct{}
	unblockRemoveReader chan struct{}
	startedOnce         sync.Once
	unblockOnce         sync.Once
}

func (*testBlockingPath) Name() string {
	return "test"
}

func (*testBlockingPath) SafeConf() *conf.Path {
	return &conf.Path{}
}

func (*testBlockingPath) ExternalCmdEnv() externalcmd.Environment {
	return nil
}

func (*testBlockingPath) RemovePublisher(defs.PathRemovePublisherReq) {}

func (p *testBlockingPath) RemoveReader(defs.PathRemoveReaderReq) {
	p.startedOnce.Do(func() {
		close(p.removeReaderStarted)
	})
	<-p.unblockRemoveReader
}

func (p *testBlockingPath) unblock() {
	p.unblockOnce.Do(func() {
		close(p.unblockRemoveReader)
	})
}

type testBlockingPathManager struct {
	path      *testBlockingPath
	added     chan struct{}
	addedOnce sync.Once
}

func (m *testBlockingPathManager) AddReader(defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
	m.addedOnce.Do(func() {
		close(m.added)
	})
	return &defs.PathAddReaderRes{Path: m.path}, nil
}

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

func TestManagerRemoveDoesNotWaitForTargetShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	path := &testBlockingPath{
		removeReaderStarted: make(chan struct{}),
		unblockRemoveReader: make(chan struct{}),
	}
	t.Cleanup(path.unblock)

	pathManager := &testBlockingPathManager{
		path:  path,
		added: make(chan struct{}),
	}

	m := &Manager{
		PathName:    "test",
		PathManager: pathManager,
		Parent:      &testLogger{},
	}
	m.Initialize(nil)

	target, err := m.Add("rtmp://" + ln.Addr().String() + "/target")
	require.NoError(t, err)

	select {
	case <-pathManager.added:
	case <-time.After(2 * time.Second):
		t.Fatal("target did not add a reader")
	}

	var conn net.Conn
	select {
	case conn = <-accepted:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("target did not connect")
	}

	removeDone := make(chan error, 1)
	go func() {
		removeDone <- m.Remove(target.ID())
	}()

	select {
	case err = <-removeDone:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Remove() is waiting for target shutdown")
	}

	select {
	case <-path.removeReaderStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("target did not remove its reader")
	}

	path.unblock()

	select {
	case <-target.done:
	case <-time.After(2 * time.Second):
		t.Fatal("target did not shut down")
	}

	m.Close()
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
