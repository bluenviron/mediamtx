package core

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	rtmpcodecs "github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

func startRTMPPushTargetServer(t *testing.T) (string, <-chan [][]byte, <-chan error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		ln.Close()
	})

	received := make(chan [][]byte, 1)
	serverErr := make(chan error, 1)

	go func() {
		nconn, acceptErr := ln.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
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
		err = r.Initialize()
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
				serverErr <- err
				return
			}
		}
	}()

	return "rtmp://" + ln.Addr().String() + "/target", received, serverErr
}

func TestPathPushTargetRTMP(t *testing.T) {
	targetURL, received, serverErr := startRTMPPushTargetServer(t)

	p, ok := newInstance(t, "api: yes\n"+
		"paths:\n"+
		"  source:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	u, err := url.Parse("rtmp://127.0.0.1:1935/source")
	require.NoError(t, err)

	source := &gortmplib.Client{
		URL:     u,
		Publish: true,
	}
	err = source.Initialize(context.Background())
	require.NoError(t, err)
	defer source.Close()

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

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	err = w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
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
	require.Equal(t, defs.APIPushTargetSourceAPI, added.Source)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case au := <-received:
			require.Contains(t, au, []byte{5, 2, 3, 4})
			return

		case err = <-serverErr:
			require.NoError(t, err)

		case <-ticker.C:
			err = w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
			require.NoError(t, err)

		case <-timer.C:
			t.Fatal("timed out waiting for RTMP pushed frame")
		}
	}
}
