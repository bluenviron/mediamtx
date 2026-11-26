package core

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCoreRTMPOnDemandReaderAllowsPublisherWithLimitOne(t *testing.T) {
	onDemandStarted := filepath.Join(t.TempDir(), "ondemand_started")
	p, ok := newInstance(t, "rtsp: no\n"+
		"hls: no\n"+
		"webrtc: no\n"+
		"srt: no\n"+
		"rtmp: yes\n"+
		"rtmpAddress: 127.0.0.1:1940\n"+
		"rtmpMaxUnauthenticatedConns: 1\n"+
		"paths:\n"+
		"  ondemand:\n"+
		"    runOnDemand: sh -c \"touch "+onDemandStarted+" && sleep 10\"\n"+
		"    runOnDemandStartTimeout: 5s\n")
	require.Equal(t, true, ok)
	defer p.Close()

	// Start an RTMP reader; it authenticates and waits for the on-demand path to become ready.
	readURL, err := url.Parse("rtmp://127.0.0.1:1940/ondemand")
	require.NoError(t, err)
	reader := &gortmplib.Client{URL: readURL}
	readerDone := make(chan error, 1)
	go func() {
		readerDone <- reader.Initialize(context.Background())
	}()
	defer func() {
		reader.Close()
	}()

	// Wait until runOnDemand has actually started (reader is authenticated and waiting in path.addReader).
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(onDemandStarted)
		return statErr == nil
	}, 2*time.Second, 10*time.Millisecond)

	// Publish over RTMP while the reader is waiting; must NOT be rejected despite limit=1.
	pubURL, err := url.Parse("rtmp://127.0.0.1:1940/ondemand")
	require.NoError(t, err)
	pub := &gortmplib.Client{URL: pubURL, Publish: true}
	require.NoError(t, pub.Initialize(context.Background()))
	defer pub.Close()

	w := &gortmplib.Writer{
		Conn: pub,
		Tracks: []*gortmplib.Track{
			{Codec: &codecs.H264{
				SPS: test.FormatH264.SPS,
				PPS: test.FormatH264.PPS,
			}},
		},
	}
	require.NoError(t, w.Initialize())
	require.NoError(t, w.WriteH264(w.Tracks[0], 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}}))

	select {
	case err = <-readerDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not become ready")
	}

	require.Equal(t, uint64(0), p.rtmpServer.RejectedConns())
}
