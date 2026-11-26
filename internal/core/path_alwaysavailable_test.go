package core

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestPathAlwaysAvailableOfflineDelay(t *testing.T) {
	p, ok := newInstance(t,
		"paths:\n"+
			"  mystream:\n"+
			"    alwaysAvailable: yes\n"+
			"    alwaysAvailableOfflineDelay: 1s\n"+
			"    alwaysAvailableTracks:\n"+
			"      - codec: H264\n")
	require.Equal(t, true, ok)
	defer p.Close()

	u, err := base.ParseURL("rtsp://localhost:8554/mystream")
	require.NoError(t, err)

	reader := gortsplib.Client{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	err = reader.Start()
	require.NoError(t, err)
	defer reader.Close()

	desc, _, err := reader.Describe(u)
	require.NoError(t, err)

	err = reader.SetupAll(desc.BaseURL, desc.Medias)
	require.NoError(t, err)

	var received atomic.Int64
	reader.OnPacketRTPAny(func(_ *description.Media, _ format.Format, _ *rtp.Packet) {
		received.Add(1)
	})

	_, err = reader.Play(nil)
	require.NoError(t, err)

	// with no source, the offline segment plays immediately
	require.Eventually(t, func() bool { return received.Load() > 0 },
		5*time.Second, 50*time.Millisecond)

	// source disconnects and comes back within the delay: no offline segment
	source, write := publishH264(t, "mystream", 90000)
	require.NoError(t, write())
	source.Close()
	time.Sleep(100 * time.Millisecond)
	received.Store(0)
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, int64(0), received.Load(), "offline segment started before the delay")

	source, _ = publishH264(t, "mystream", 90000)
	time.Sleep(1500 * time.Millisecond)
	require.Equal(t, int64(0), received.Load(), "offline segment started although the source came back")

	// source stays away: offline segment starts after the delay
	source.Close()
	require.Eventually(t, func() bool { return received.Load() > 0 },
		3*time.Second, 50*time.Millisecond, "offline segment did not start after the delay")
}
