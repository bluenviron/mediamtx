package rtsp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type testServerHandler struct {
	announced chan *gortsplib.ServerHandlerOnAnnounceCtx
	received  chan []byte
}

func (h *testServerHandler) OnAnnounce(
	ctx *gortsplib.ServerHandlerOnAnnounceCtx,
) (*base.Response, error) {
	h.announced <- ctx
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func (h *testServerHandler) OnSetup(
	*gortsplib.ServerHandlerOnSetupCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, nil, nil
}

func (h *testServerHandler) OnRecord(
	ctx *gortsplib.ServerHandlerOnRecordCtx,
) (*base.Response, error) {
	ctx.Session.OnPacketRTPAny(func(_ *description.Media, _ format.Format, pkt *rtp.Packet) {
		h.received <- append([]byte(nil), pkt.Payload...)
	})
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func TestDest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	handler := &testServerHandler{
		announced: make(chan *gortsplib.ServerHandlerOnAnnounceCtx, 1),
		received:  make(chan []byte, 1),
	}
	server := &gortsplib.Server{
		RTSPAddress: "unused",
		Handler:     handler,
		Listen: func(string, string) (net.Listener, error) {
			return ln, nil
		},
	}
	require.NoError(t, server.Start())
	defer server.Close()

	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH264},
	}}}
	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: true,
	}
	require.NoError(t, subStream.Initialize())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := &Dest{
		Stream:       strm,
		Dest:         "rtsp://" + ln.Addr().String() + "/stream",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		Parent:       test.NilLogger,
	}

	done := make(chan error, 1)
	go func() {
		done <- dest.Run(ctx)
	}()

	select {
	case announced := <-handler.announced:
		require.Equal(t, "/stream", announced.Path)
		require.Len(t, announced.Description.Medias, 1)
		_, ok := announced.Description.Medias[0].Formats[0].(*format.H264)
		require.True(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTSP ANNOUNCE")
	}

	strm.WaitForReaders()
	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		NTP: time.Now(),
		PTS: 0,
		RTPPackets: []*rtp.Packet{{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      456,
				SSRC:           789,
			},
			Payload: []byte{5, 1},
		}},
	})

	select {
	case payload := <-handler.received:
		require.Equal(t, []byte{5, 1}, payload)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTP packet")
	}

	require.Eventually(t, func() bool {
		return dest.OutboundBytes() > 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case runErr := <-done:
		require.Error(t, runErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTSP destination to stop")
	}
}
