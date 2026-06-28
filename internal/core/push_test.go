package core

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	rtmpcodecs "github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func startRTMPPushTargetServer(t *testing.T) (string, <-chan [][]byte, <-chan error) {
	ready := &atomic.Bool{}
	ready.Store(true)
	u, received, _, serverErr := startRTMPPushTargetServerControlled(t, ready)
	return u, received, serverErr
}

func startRTMPPushTargetServerControlled(
	t *testing.T,
	ready *atomic.Bool,
) (string, <-chan [][]byte, <-chan struct{}, <-chan error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		ln.Close()
	})

	received := make(chan [][]byte, 16)
	connOpened := make(chan struct{}, 16)
	serverErr := make(chan error, 16)

	go func() {
		for {
			nconn, acceptErr := ln.Accept()
			if acceptErr != nil {
				select {
				case <-done:
				default:
					serverErr <- acceptErr
				}
				return
			}

			if !ready.Load() {
				nconn.Close()
				continue
			}

			select {
			case connOpened <- struct{}{}:
			default:
			}

			go handleRTMPPushTargetConn(nconn, received, serverErr)
		}
	}()

	return "rtmp://" + ln.Addr().String() + "/target", received, connOpened, serverErr
}

func handleRTMPPushTargetConn(nconn net.Conn, received chan<- [][]byte, serverErr chan<- error) {
	defer nconn.Close()

	deadlineErr := nconn.SetDeadline(time.Now().Add(10 * time.Second))
	if deadlineErr != nil {
		serverErr <- deadlineErr
		return
	}

	conn := &gortmplib.ServerConn{RW: nconn}
	initErr := conn.Initialize()
	if initErr != nil {
		serverErr <- initErr
		return
	}

	acceptConnErr := conn.Accept()
	if acceptConnErr != nil {
		serverErr <- acceptConnErr
		return
	}

	if !conn.Publish {
		serverErr <- fmt.Errorf("connection is not publishing")
		return
	}
	if conn.URL.Path != "/target" {
		serverErr <- fmt.Errorf("unexpected path: %s", conn.URL.Path)
		return
	}

	r := &gortmplib.Reader{Conn: conn}
	err := r.Initialize()
	if err != nil {
		serverErr <- err
		return
	}

	tracks := r.Tracks()
	if len(tracks) != 1 {
		serverErr <- fmt.Errorf("unexpected track count: %d", len(tracks))
		return
	}
	if _, ok := tracks[0].Codec.(*rtmpcodecs.H264); !ok {
		serverErr <- fmt.Errorf("unexpected codec: %T", tracks[0].Codec)
		return
	}

	r.OnDataH264(tracks[0], func(_ time.Duration, _ time.Duration, au [][]byte) {
		for _, nalu := range au {
			if bytes.Equal(nalu, []byte{5, 2, 3, 4}) {
				select {
				case received <- au:
				default:
				}
			}
		}
	})

	for {
		err = r.Read()
		if err != nil {
			return
		}
	}
}

func startRTMPPublisher(
	t *testing.T,
	path string,
) (*gortmplib.Client, *gortmplib.Writer, *gortmplib.Track) {
	u, err := url.Parse("rtmp://127.0.0.1:1935/" + path)
	require.NoError(t, err)

	source := &gortmplib.Client{
		URL:     u,
		Publish: true,
	}
	err = source.Initialize(context.Background())
	require.NoError(t, err)

	track := &gortmplib.Track{
		Codec: &rtmpcodecs.H264{
			SPS: test.FormatH264.SPS,
			PPS: test.FormatH264.PPS,
		},
	}

	w := &gortmplib.Writer{
		Conn:   source,
		Tracks: []*gortmplib.Track{track},
	}
	err = w.Initialize()
	require.NoError(t, err)

	return source, w, track
}

func waitRTMPPushTargetFrame(
	t *testing.T,
	w *gortmplib.Writer,
	track *gortmplib.Track,
	received <-chan [][]byte,
	serverErr <-chan error,
) {
	t.Helper()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	for {
		select {
		case au := <-received:
			require.Contains(t, au, []byte{5, 2, 3, 4})
			return

		case err := <-serverErr:
			require.NoError(t, err)

		case <-ticker.C:
			err := w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
			require.NoError(t, err)

		case <-timer.C:
			t.Fatal("timed out waiting for RTMP pushed frame")
		}
	}
}

func TestPathPushTargetRTMP(t *testing.T) {
	targetURL, received, serverErr := startRTMPPushTargetServer(t)

	p, ok := newInstance(t, "api: yes\n"+
		"paths:\n"+
		"  source:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source, w, track := startRTMPPublisher(t, "source")
	defer source.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	err := w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var path struct {
			Ready bool `json:"ready"`
		}
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/source", nil, &path)
		return path.Ready
	}, 5*time.Second, 100*time.Millisecond)

	var added defs.APIPushTarget
	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/paths/pushtargets/add/source",
		defs.APIPushTargetAdd{URL: targetURL}, &added)
	require.Equal(t, targetURL, added.URL)
	require.Equal(t, defs.APIPushTargetProtocolRTMP, added.Protocol)
	require.Equal(t, defs.APIPushTargetSourceAPI, added.Source)

	waitRTMPPushTargetFrame(t, w, track, received, serverErr)

	require.Eventually(t, func() bool {
		var item defs.APIPushTarget
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/pushtargets/get/"+added.ID.String()+"/source", nil, &item)
		return item.State == defs.APIPushTargetStatePushing &&
			item.Protocol == defs.APIPushTargetProtocolRTMP &&
			item.OutboundBytes > 0 &&
			item.BytesSent == item.OutboundBytes
	}, 5*time.Second, 100*time.Millisecond)
}

func TestPathPushTargetRTMPReconnectsAfterSourceUnavailable(t *testing.T) {
	targetURL, received, serverErr := startRTMPPushTargetServer(t)

	p, ok := newInstance(t, "api: yes\n"+
		"paths:\n"+
		"  source:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var added defs.APIPushTarget
	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/paths/pushtargets/add/source",
		defs.APIPushTargetAdd{URL: targetURL}, &added)

	require.Eventually(t, func() bool {
		var list defs.APIPushTargetList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/pushtargets/list/source", nil, &list)
		return list.ItemCount == 1 &&
			list.Items[0].ID == added.ID &&
			list.Items[0].State == defs.APIPushTargetStateError
	}, 7*time.Second, 100*time.Millisecond)

	source, w, track := startRTMPPublisher(t, "source")
	waitRTMPPushTargetFrame(t, w, track, received, serverErr)
	source.Close()

	require.Eventually(t, func() bool {
		var list defs.APIPushTargetList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/pushtargets/list/source", nil, &list)
		return list.ItemCount == 1 && list.Items[0].ID == added.ID
	}, 5*time.Second, 100*time.Millisecond)

	for {
		select {
		case <-received:
		default:
			goto drained
		}
	}

drained:
	source, w, track = startRTMPPublisher(t, "source")
	defer source.Close()

	waitRTMPPushTargetFrame(t, w, track, received, serverErr)

	var item defs.APIPushTarget
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/pushtargets/get/"+added.ID.String()+"/source", nil, &item)
	require.Equal(t, added.ID, item.ID)
	require.Equal(t, defs.APIPushTargetStatePushing, item.State)
	require.Greater(t, item.OutboundBytes, uint64(0))
}

func TestPathPushTargetRTMPReconnectsAfterDestinationUnavailable(t *testing.T) {
	ready := &atomic.Bool{}
	targetURL, received, _, serverErr := startRTMPPushTargetServerControlled(t, ready)

	p, ok := newInstance(t, "api: yes\n"+
		"paths:\n"+
		"  source:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source, w, track := startRTMPPublisher(t, "source")
	defer source.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var added defs.APIPushTarget
	httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/paths/pushtargets/add/source",
		defs.APIPushTargetAdd{URL: targetURL}, &added)

	require.Eventually(t, func() bool {
		var list defs.APIPushTargetList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/pushtargets/list/source", nil, &list)
		return list.ItemCount == 1 &&
			list.Items[0].ID == added.ID &&
			list.Items[0].State == defs.APIPushTargetStateError
	}, 7*time.Second, 100*time.Millisecond)

	ready.Store(true)
	waitRTMPPushTargetFrame(t, w, track, received, serverErr)

	var item defs.APIPushTarget
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/pushtargets/get/"+added.ID.String()+"/source", nil, &item)
	require.Equal(t, added.ID, item.ID)
	require.Equal(t, defs.APIPushTargetStatePushing, item.State)
	require.Equal(t, defs.APIPushTargetProtocolRTMP, item.Protocol)
	require.Greater(t, item.OutboundBytes, uint64(0))
}
