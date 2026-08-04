package forward

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

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

type testLogger struct {
	entries chan string
}

func (l *testLogger) Log(_ logger.Level, format string, args ...any) {
	if l.entries != nil {
		l.entries <- fmt.Sprintf(format, args...)
	}
}

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
	logEntries := make(chan string, 32)
	m := &Manager{
		PathName: "test",
		Forward: conf.Forward{
			{Dest: "rtmp://localhost/app/stream"},
			{Dest: "rtsp://localhost:8554/stream"},
		},
		PathManager: &testPathManager{},
		Parent:      &testLogger{entries: logEntries},
	}
	m.Initialize()
	defer m.Close()

	list := m.List()
	require.Len(t, list.Items, 2)
	require.Equal(t, "rtmp://localhost/app/stream", list.Items[0].Dest)
	require.Equal(t, 1, list.Items[0].Pos)
	require.Equal(t, "rtsp://localhost:8554/stream", list.Items[1].Dest)
	require.Equal(t, 2, list.Items[1].Pos)
	rtmpID := list.Items[0].ID
	rtspID := list.Items[1].ID

	m.destHandlers[0].Log(logger.Info, "marker")
	expectedLog := "[forward] [dest 1 " + hex.EncodeToString(rtmpID[:4]) + "] marker"
	require.Eventually(t, func() bool {
		select {
		case entry := <-logEntries:
			return strings.Contains(entry, expectedLog)
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	m.ReloadConf(conf.Forward{
		{Dest: "rtsp://localhost:8554/stream"},
		{Dest: "srt://localhost:8890?streamid=publish:test"},
		{Dest: "rtmp://localhost/app/stream"},
	})
	list = m.List()
	require.Len(t, list.Items, 3)
	require.Equal(t, rtspID, list.Items[0].ID)
	require.Equal(t, 1, list.Items[0].Pos)
	require.Equal(t, "srt://localhost:8890?streamid=publish:test", list.Items[1].Dest)
	require.Equal(t, 2, list.Items[1].Pos)
	require.Equal(t, rtmpID, list.Items[2].ID)
	require.Equal(t, 3, list.Items[2].Pos)
	m.destHandlers[2].Log(logger.Info, "marker after reload")
	expectedLog = "[forward] [dest 3 " + hex.EncodeToString(rtmpID[:4]) + "] marker after reload"
	require.Eventually(t, func() bool {
		select {
		case entry := <-logEntries:
			return strings.Contains(entry, expectedLog)
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	item, err := m.Get(rtmpID)
	require.NoError(t, err)
	require.Equal(t, 3, item.Pos)

	_, err = m.Get(uuid.New())
	require.ErrorIs(t, err, ErrDestNotFound)

	m.ReloadConf(conf.Forward{{Dest: "srt://localhost:8890?streamid=publish:test"}})
	list = m.List()
	require.Len(t, list.Items, 1)
	require.Equal(t, "srt://localhost:8890?streamid=publish:test", list.Items[0].Dest)
	require.Equal(t, 1, list.Items[0].Pos)
}

func TestManagerReloadDoesNotWaitForDestShutdown(t *testing.T) {
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
		Forward:     conf.Forward{{Dest: "rtmp://" + ln.Addr().String() + "/dest"}},
		PathManager: pathManager,
		Parent:      &testLogger{},
	}
	m.Initialize()
	dest := m.destHandlers[0]

	select {
	case <-pathManager.added:
	case <-time.After(2 * time.Second):
		t.Fatal("dest did not add a reader")
	}

	var conn net.Conn
	select {
	case conn = <-accepted:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("dest did not connect")
	}

	reloadDone := make(chan struct{})
	go func() {
		m.ReloadConf(nil)
		close(reloadDone)
	}()

	select {
	case <-reloadDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ReloadConf() is waiting for dest shutdown")
	}

	select {
	case <-path.removeReaderStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("dest did not remove its reader")
	}

	path.unblock()

	select {
	case <-dest.done:
	case <-time.After(2 * time.Second):
		t.Fatal("dest did not shut down")
	}

	m.Close()
}
