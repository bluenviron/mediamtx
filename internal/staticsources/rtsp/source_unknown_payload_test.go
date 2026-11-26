package rtsp_test

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/staticsources/rtsp"
	"github.com/bluenviron/mediamtx/internal/test"
)

type unknownPayloadSourceParent struct {
	test.StaticSourceParent
	ready chan *description.Session
}

func (p *unknownPayloadSourceParent) SetReady(
	req defs.PathSourceStaticSetReadyReq,
) defs.PathSourceStaticSetReadyRes {
	p.ready <- req.Desc
	return p.StaticSourceParent.SetReady(req)
}

func TestUnknownPayloadTypeAPISerialization(t *testing.T) {
	for _, value := range []uint64{0, 3} {
		payload, err := json.Marshal(defs.APIStaticSourceTypeSpecificRTSP{InboundRTPPacketsUnknownPayloadType: value})
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &fields))
		got, present := fields["inboundRTPPacketsUnknownPayloadType"]
		require.True(t, present)
		require.Equal(t, strconv.FormatUint(value, 10), string(got))
	}
}

// A camera sending RTP with a payload type missing from its SDP must show in the source
// info as Unknown == InError, so a health check can subtract it.
func TestSourceUnknownPayloadTypeStats(t *testing.T) {
	const unknownCount = 6

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	// The server describes video only, but streams PT 50 as well.
	extra := &format.Generic{PayloadTyp: 50, RTPMa: "private/90000"}
	require.NoError(t, extra.Init())
	videoOnly := test.UniqueMediaH264()
	full := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{videoOnly.Formats[0], extra},
	}

	var describeStream, setupStream *gortsplib.ServerStream
	server := gortsplib.Server{
		RTSPAddress: addr,
		Handler: &testServer{
			onDescribe: func(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{StatusCode: base.StatusOK}, describeStream, nil
			},
			onSetup: func(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{StatusCode: base.StatusOK}, setupStream, nil
			},
			onPlay: func(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{StatusCode: base.StatusOK}, nil
			},
		},
	}
	require.NoError(t, server.Start())
	defer server.Close()

	describeStream = &gortsplib.ServerStream{
		Server: &server, Desc: &description.Session{Medias: []*description.Media{videoOnly}},
	}
	require.NoError(t, describeStream.Initialize())
	defer describeStream.Close()
	setupStream = &gortsplib.ServerStream{
		Server: &server, Desc: &description.Session{Medias: []*description.Media{full}},
	}
	require.NoError(t, setupStream.Initialize())
	defer setupStream.Close()

	parent := &unknownPayloadSourceParent{ready: make(chan *description.Session, 1)}
	parent.Initialize()
	defer parent.Close()

	source := &rtsp.Source{
		ReadTimeout: conf.Duration(5 * time.Second), WriteTimeout: conf.Duration(5 * time.Second),
		WriteQueueSize: 512, Parent: parent,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	defer func() { cancel(); <-done }()
	go func() {
		done <- source.Run(defs.StaticSourceRunParams{
			Context: ctx, ResolvedSource: "rtsp://" + addr + "/camera",
			Conf: &conf.Path{
				RTSPTransport:          conf.RTSPTransport{Protocol: new(gortsplib.ProtocolTCP)},
				RTSPUDPSourcePortRange: []uint{32768, 60999},
			},
		})
	}()

	select {
	case <-parent.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("source did not become ready")
	}

	// Ready is signalled before PLAY completes, so keep sending until a packet lands.
	seq := uint16(0)
	require.Eventually(t, func() bool {
		seq++
		pkt := &rtp.Packet{
			Header:  rtp.Header{Version: 2, PayloadType: 50, SequenceNumber: seq, SSRC: 0x38F27A2F},
			Payload: []byte{1, 2, 3, 4},
		}
		require.NoError(t, setupStream.WritePacketRTP(full, pkt))
		ts, ok := source.Info().TypeSpecific.(*defs.APIStaticSourceTypeSpecificRTSP)
		return ok && ts.InboundRTPPacketsInError >= unknownCount
	}, 5*time.Second, 20*time.Millisecond)

	// Nothing else is sending, so let any packet already in flight land before reading.
	time.Sleep(200 * time.Millisecond)
	ts, ok := source.Info().TypeSpecific.(*defs.APIStaticSourceTypeSpecificRTSP)
	require.True(t, ok)
	require.Greater(t, ts.InboundRTPPacketsUnknownPayloadType, uint64(0))
	require.Equal(t, ts.InboundRTPPacketsInError, ts.InboundRTPPacketsUnknownPayloadType)
}
