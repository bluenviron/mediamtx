package rtsp

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/test"
)

// a fresh source, before connecting, reports nil stats.
func TestSourceStatsEmpty(t *testing.T) {
	so := &Source{}
	require.Nil(t, so.SourceStats())
}

func TestSourceStats(t *testing.T) {
	var strm *gortsplib.ServerStream

	media0 := test.UniqueMediaH264()

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(_ *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, strm, nil
			},
			onSetup: func(_ *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, strm, nil
			},
			onPlay: func(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				go func() {
					time.Sleep(100 * time.Millisecond)
					err := strm.WritePacketRTP(media0, &rtp.Packet{
						Header: rtp.Header{
							Version:        0x02,
							PayloadType:    96,
							SequenceNumber: 57899,
							Timestamp:      345234345,
							SSRC:           978651231,
							Marker:         true,
						},
						Payload: []byte{5, 1, 2, 3, 4},
					})
					require.NoError(t, err)
				}()

				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "127.0.0.1:8557",
	}

	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	strm = &gortsplib.ServerStream{
		Server: &s,
		Desc:   &description.Session{Medias: []*description.Media{media0}},
	}
	err = strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	var sp conf.RTSPTransport
	sp.UnmarshalJSON([]byte(`"tcp"`)) //nolint:errcheck

	p := &test.StaticSourceParent{}
	p.Initialize()
	defer p.Close()

	so := &Source{
		ReadTimeout:    conf.Duration(10 * time.Second),
		WriteTimeout:   conf.Duration(10 * time.Second),
		WriteQueueSize: 2048,
		Parent:         p,
	}

	done := make(chan struct{})
	defer func() { <-done }()

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	reloadConf := make(chan *conf.Path)

	go func() {
		so.Run(defs.StaticSourceRunParams{ //nolint:errcheck
			Context:        ctx,
			ResolvedSource: "rtsp://127.0.0.1:8557/teststream",
			Conf: &conf.Path{
				RTSPTransport:          sp,
				RTSPUDPSourcePortRange: []uint{10000, 65535},
			},
			ReloadConf: reloadConf,
		})
		close(done)
	}()

	<-p.Unit

	// stats are derived from RTCP/session counters that update
	// asynchronously, so poll until the received packet is accounted for.
	require.Eventually(t, func() bool {
		st := so.SourceStats()
		if st == nil {
			return false
		}
		rst, ok := st.(*defs.RTSPSourceStats)
		if !ok {
			return false
		}
		return rst.Jitter != nil && rst.PacketsReceived >= 1
	}, 5*time.Second, 50*time.Millisecond)

	// the source must be listening on ReloadConf
	reloadConf <- nil
}
