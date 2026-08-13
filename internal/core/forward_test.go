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
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func startRTMPForwardServer(t *testing.T) (string, <-chan [][]byte, <-chan error) {
	ready := &atomic.Bool{}
	ready.Store(true)
	u, received, _, serverErr := startRTMPForwardServerControlled(t, ready)
	return u, received, serverErr
}

func startRTMPForwardServerControlled(
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

			go handleRTMPForwardConn(nconn, received, serverErr)
		}
	}()

	return "rtmp://" + ln.Addr().String() + "/dest", received, connOpened, serverErr
}

func handleRTMPForwardConn(nconn net.Conn, received chan<- [][]byte, serverErr chan<- error) {
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

	acceptConnErr := conn.AcceptConn()
	if acceptConnErr != nil {
		serverErr <- acceptConnErr
		return
	}

	if !conn.Publish {
		serverErr <- fmt.Errorf("connection is not publishing")
		return
	}
	if conn.URL.Path != "/dest" {
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

func waitRTMPForwardFrame(
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
			t.Fatal("timed out waiting for RTMP forwarded frame")
		}
	}
}

func TestPathForwardRTMP(t *testing.T) {
	dest, received, serverErr := startRTMPForwardServer(t)

	p, ok := newInstance(t, "api: yes\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+dest+"\n")
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

	var list defs.APIForwardDestList
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
	require.Len(t, list.Items, 1)
	added := list.Items[0]
	require.Equal(t, dest, added.Conf.Dest)
	require.Equal(t, defs.APIForwardDestProtocolRTMP, added.Protocol)
	require.Equal(t, 1, added.Pos)

	waitRTMPForwardFrame(t, w, track, received, serverErr)

	require.Eventually(t, func() bool {
		var item defs.APIForwardDest
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/get?path=source&id="+added.ID.String(), nil, &item)
		return item.State == defs.APIForwardDestStateForwarding &&
			item.Protocol == defs.APIForwardDestProtocolRTMP &&
			item.OutboundBytes > 0
	}, 5*time.Second, 100*time.Millisecond)
}

func TestPathForwardRTMPReconnectsAfterSourceUnavailable(t *testing.T) {
	dest, received, serverErr := startRTMPForwardServer(t)

	p, ok := newInstance(t, "api: yes\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+dest+"\n")
	require.Equal(t, true, ok)
	defer p.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var id uuid.UUID
	require.Eventually(t, func() bool {
		var list defs.APIForwardDestList
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
		if list.ItemCount != 1 || list.Items[0].State != defs.APIForwardDestStateIdle {
			return false
		}
		id = list.Items[0].ID
		return true
	}, 7*time.Second, 100*time.Millisecond)

	source, w, track := startRTMPPublisher(t, "source")
	waitRTMPForwardFrame(t, w, track, received, serverErr)
	source.Close()

	require.Eventually(t, func() bool {
		var list defs.APIForwardDestList
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
		return list.ItemCount == 1 && list.Items[0].ID == id
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

	waitRTMPForwardFrame(t, w, track, received, serverErr)

	var item defs.APIForwardDest
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/get?path=source&id="+id.String(), nil, &item)
	require.Equal(t, id, item.ID)
	require.Equal(t, defs.APIForwardDestStateForwarding, item.State)
	require.Greater(t, item.OutboundBytes, uint64(0))
}

func TestPathForwardRTMPReconnectsAfterDestinationUnavailable(t *testing.T) {
	ready := &atomic.Bool{}
	dest, received, _, serverErr := startRTMPForwardServerControlled(t, ready)

	p, ok := newInstance(t, "api: yes\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+dest+"\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source, w, track := startRTMPPublisher(t, "source")
	defer source.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var id uuid.UUID
	require.Eventually(t, func() bool {
		var list defs.APIForwardDestList
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
		if list.ItemCount != 1 || list.Items[0].State != defs.APIForwardDestStateIdle {
			return false
		}
		id = list.Items[0].ID
		return true
	}, 7*time.Second, 100*time.Millisecond)

	ready.Store(true)
	waitRTMPForwardFrame(t, w, track, received, serverErr)

	var item defs.APIForwardDest
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/get?path=source&id="+id.String(), nil, &item)
	require.Equal(t, id, item.ID)
	require.Equal(t, defs.APIForwardDestStateForwarding, item.State)
	require.Equal(t, defs.APIForwardDestProtocolRTMP, item.Protocol)
	require.Greater(t, item.OutboundBytes, uint64(0))
}
