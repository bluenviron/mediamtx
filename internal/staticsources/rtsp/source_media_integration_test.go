package rtsp_test

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/staticsources/rtsp"
	"github.com/bluenviron/mediamtx/internal/test"
)

type filteredSourceParent struct {
	test.StaticSourceParent
	ready chan *description.Session
}

func (p *filteredSourceParent) SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes {
	p.ready <- req.Desc
	return p.StaticSourceParent.SetReady(req)
}

func TestSourceMediaFilterBeforeSetup(t *testing.T) {
	for _, wanted := range []string{"video", "audio"} {
		t.Run(wanted, func(t *testing.T) {
			var serverStream *gortsplib.ServerStream
			setups := 0
			played := make(chan int, 1)
			server := gortsplib.Server{
				RTSPAddress: "127.0.0.1:8557",
				Handler: &testServer{
					onDescribe: func(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
						return &base.Response{StatusCode: base.StatusOK}, serverStream, nil
					},
					onSetup: func(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
						setups++
						return &base.Response{StatusCode: base.StatusOK}, serverStream, nil
					},
					onPlay: func(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
						played <- setups
						return &base.Response{StatusCode: base.StatusOK}, nil
					},
				},
			}
			require.NoError(t, server.Start())
			defer server.Close()
			serverStream = &gortsplib.ServerStream{
				Server: &server,
				Desc: &description.Session{Medias: []*description.Media{
					test.UniqueMediaH264(),
					{Type: description.MediaTypeAudio, Formats: []format.Format{&format.Opus{PayloadTyp: 97, ChannelCount: 1}}},
				}},
			}
			require.NoError(t, serverStream.Initialize())
			defer serverStream.Close()
			parent := &filteredSourceParent{ready: make(chan *description.Session, 1)}
			parent.Initialize()
			defer parent.Close()
			source := &rtsp.Source{
				ReadTimeout: conf.Duration(5 * time.Second), WriteTimeout: conf.Duration(5 * time.Second),
				WriteQueueSize: 512, Parent: parent,
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			defer func() {
				cancel()
				require.NoError(t, <-done)
			}()
			go func() {
				done <- source.Run(defs.StaticSourceRunParams{
					Context: ctx, ResolvedSource: "rtsp://127.0.0.1:8557/camera",
					Conf: &conf.Path{
						RTSPTransport:        conf.RTSPTransport{Protocol: new(gortsplib.ProtocolTCP)},
						RTSPSourceMediaTypes: new([]string{wanted}), RTSPUDPSourcePortRange: []uint{32768, 60999},
					},
				})
			}()
			select {
			case desc := <-parent.ready:
				require.Len(t, desc.Medias, 1)
				require.Equal(t, description.MediaType(wanted), desc.Medias[0].Type)
			case <-time.After(5 * time.Second):
				t.Fatal("source did not become ready")
			}
			select {
			case count := <-played:
				require.Equal(t, 1, count, "excluded media must never be SETUP")
			case <-time.After(5 * time.Second):
				t.Fatal("source did not PLAY")
			}
		})
	}
}
