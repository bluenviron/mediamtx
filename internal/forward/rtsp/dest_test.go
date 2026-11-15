package rtsp_test

import (
	"bytes"
	"context"
	"net"
	"sync"
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
	"github.com/bluenviron/mediamtx/internal/forward/rtsp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type testServerHandler struct {
	conn      *gortsplib.ServerConn
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
	for _, scheme := range []string{"rtsp", "rtsp+ws"} {
		t.Run(scheme, func(t *testing.T) {
			testDest(t, scheme)
		})
	}
}

func testDest(t *testing.T, scheme string) {
	t.Helper()

	ln0, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ln := &recordingListener{Listener: ln0}

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
	dest := &rtsp.Dest{
		Stream:       strm,
		Dest:         scheme + "://" + ln.Addr().String() + "/stream?key=value",
		DestHeaders:  map[string]string{"X-Custom": "myvalue"},
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
		handler.conn = announced.Conn
		require.Equal(t, "/stream", announced.Path)
		require.Len(t, announced.Description.Medias, 1)
		_, ok := announced.Description.Medias[0].Formats[0].(*format.H264)
		require.True(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTSP ANNOUNCE")
	}

	if scheme == "rtsp+ws" {
		require.Equal(t, gortsplib.TunnelWebSocket, handler.conn.Transport().Tunnel)
		received := ln.received()
		require.Contains(t, received, "GET /stream?key=value HTTP/1.1\r\n")
		require.Contains(t, received, "X-Custom: myvalue\r\n")
	}

	strm.WaitForReaders()
	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		NTP: time.Now(),
		PTS: 0,
		RTPPackets: []*rtp.Packet{{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      456,
			SSRC:           789,
			Payload:        []byte{5, 1},
		}},
	})

	select {
	case payload := <-handler.received:
		require.Equal(t, []byte{5, 1}, payload)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTP packet")
	}

	require.Eventually(t, func() bool {
		return dest.Info().OutboundBytes > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		info := dest.Info()
		typeSpecific, ok := info.TypeSpecific.(*defs.APIForwardDestTypeSpecificRTSP)
		return ok && typeSpecific.RemoteAddr == ln.Addr().String() && typeSpecific.Transport != "" &&
			typeSpecific.OutboundBytes == info.OutboundBytes && typeSpecific.OutboundRTPPackets > 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case runErr := <-done:
		require.Error(t, runErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTSP destination to stop")
	}
}

func TestDestServerDisconnect(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := &rtsp.Dest{
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

	strm.WaitForReaders()
	server.Close()

	select {
	case runErr := <-done:
		require.Error(t, runErr)
		require.NotEqual(t, "terminated", runErr.Error())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTSP destination to detect server disconnect")
	}
}

func TestDestCancelWhileStarting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan struct{}, 1)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)

		nconn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer nconn.Close()

		accepted <- struct{}{}
		<-time.After(5 * time.Second)
	}()

	desc := &description.Session{Medias: []*description.Media{}}
	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := &rtsp.Dest{
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
	case <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RTSP connection")
	}

	cancel()

	select {
	case runErr := <-done:
		require.EqualError(t, runErr, "terminated")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RTSP destination to stop")
	}

	<-serverDone
}

type recordingConn struct {
	net.Conn
	mutex *sync.Mutex
	buf   *bytes.Buffer
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.mutex.Lock()
	c.buf.Write(p[:n])
	c.mutex.Unlock()
	return n, err
}

type recordingListener struct {
	net.Listener
	mutex sync.Mutex
	buf   bytes.Buffer
}

func (l *recordingListener) Accept() (net.Conn, error) {
	nconn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &recordingConn{Conn: nconn, mutex: &l.mutex, buf: &l.buf}, nil
}

func (l *recordingListener) received() string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.buf.String()
}
