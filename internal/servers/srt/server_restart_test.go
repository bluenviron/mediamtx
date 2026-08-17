package srt

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	srt "github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"
)

// fakeListener is a minimal in-memory implementation of srt.Listener used to
// drive the SRT server's accept-error/restart code paths without touching the
// network.
type fakeListener struct {
	// acceptCh, when non-nil, supplies values returned by Accept2(). When
	// closed (or drained), Accept2() returns srt.ErrListenerClosed.
	acceptCh chan acceptResult

	closeOnce sync.Once
	closed    chan struct{}
}

type acceptResult struct {
	req srt.ConnRequest
	err error
}

func newFakeListener() *fakeListener {
	return &fakeListener{
		acceptCh: make(chan acceptResult, 4),
		closed:   make(chan struct{}),
	}
}

func (f *fakeListener) Accept2() (srt.ConnRequest, error) {
	select {
	case <-f.closed:
		return nil, srt.ErrListenerClosed
	case res, ok := <-f.acceptCh:
		if !ok {
			return nil, srt.ErrListenerClosed
		}
		return res.req, res.err
	}
}

func (f *fakeListener) Accept(_ srt.AcceptFunc) (srt.Conn, srt.ConnType, error) {
	return nil, srt.REJECT, errors.New("Accept is not used in tests")
}

func (f *fakeListener) Close() {
	f.closeOnce.Do(func() {
		close(f.closed)
	})
}

func (f *fakeListener) Addr() net.Addr {
	a, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	return a
}

// withListenerFactory swaps the package-level srtListen indirection for the
// duration of a test and restores it on cleanup.
func withListenerFactory(t *testing.T, fn func(network, address string, config srt.Config) (srt.Listener, error)) {
	t.Helper()
	orig := srtListen
	srtListen = fn
	t.Cleanup(func() { srtListen = orig })
}

// withFastBackoff shortens the listener restart backoff so tests run quickly,
// and restores the production values on cleanup.
func withFastBackoff(t *testing.T) {
	t.Helper()
	origBase, origMax := listenerRestartBaseDelay, listenerRestartMaxDelay
	listenerRestartBaseDelay = 5 * time.Millisecond
	listenerRestartMaxDelay = 50 * time.Millisecond
	t.Cleanup(func() {
		listenerRestartBaseDelay = origBase
		listenerRestartMaxDelay = origMax
	})
}

// newRestartTestServer constructs a minimal *Server suitable for restart
// tests. It does not need a path manager because no connection requests are
// produced by the fake listeners used here.
func newRestartTestServer() *Server {
	return &Server{
		Address:           "127.0.0.1:0",
		ReadTimeout:       conf.Duration(10 * time.Second),
		WriteTimeout:      conf.Duration(10 * time.Second),
		UDPMaxPayloadSize: 1472,
		Parent:            test.NilLogger,
	}
}

func TestServerRestartsOnTransientListenerError(t *testing.T) {
	withFastBackoff(t)

	first := newFakeListener()
	first.acceptCh <- acceptResult{err: errors.New("read udp: network is unreachable")}

	second := newFakeListener()
	secondCreated := make(chan struct{})

	var calls int32
	withListenerFactory(t, func(_, _ string, _ srt.Config) (srt.Listener, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			return first, nil
		case 2:
			close(secondCreated)
			return second, nil
		default:
			return nil, errors.New("unexpected extra Listen call")
		}
	})

	s := newRestartTestServer()
	require.NoError(t, s.Initialize())
	defer s.Close()

	select {
	case <-secondCreated:
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not restart its listener after a transient error")
	}

	require.EqualValues(t, 2, atomic.LoadInt32(&calls))
}

func TestServerDoesNotRestartOnErrListenerClosed(t *testing.T) {
	withFastBackoff(t)

	first := newFakeListener()
	first.acceptCh <- acceptResult{err: srt.ErrListenerClosed}

	var calls int32
	withListenerFactory(t, func(_, _ string, _ srt.Config) (srt.Listener, error) {
		atomic.AddInt32(&calls, 1)
		return first, nil
	})

	s := newRestartTestServer()
	require.NoError(t, s.Initialize())

	// The server should shut its run loop down on its own once
	// ErrListenerClosed propagates. Close() must then return promptly.
	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatalf("server.Close() did not return after ErrListenerClosed")
	}

	require.EqualValues(t, 1, atomic.LoadInt32(&calls), "no listener restart should be attempted")
}

func TestServerRestartGivesUpOnContextCancel(t *testing.T) {
	withFastBackoff(t)
	// Make every retry slow enough that we are guaranteed to be inside the
	// backoff sleep when Close() cancels the context.
	listenerRestartBaseDelay = 200 * time.Millisecond
	listenerRestartMaxDelay = 200 * time.Millisecond

	first := newFakeListener()
	first.acceptCh <- acceptResult{err: errors.New("transient failure")}

	restartAttempted := make(chan struct{}, 1)

	var calls int32
	withListenerFactory(t, func(_, _ string, _ srt.Config) (srt.Listener, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return first, nil
		}
		select {
		case restartAttempted <- struct{}{}:
		default:
		}
		return nil, errors.New("listen permanently broken")
	})

	s := newRestartTestServer()
	require.NoError(t, s.Initialize())

	select {
	case <-restartAttempted:
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not attempt at least one listener restart")
	}

	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatalf("server.Close() did not return while restart loop was active")
	}
}
