package moq_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/test"
)

const testVersion = "moqt-19"

type publishRequest struct {
	message  controlmessage.Publish
	subGroup subgroup.SubGroup
}

type testMoqServer struct {
	address       string
	trackAlias    uint64
	subgroupAlias uint64
	publish       chan publishRequest

	ctx       context.Context
	ctxCancel func()
	listener  *quic.Listener
	err       chan error
}

func newTestMoqServerWithProtocols(
	t *testing.T,
	trackAlias uint64,
	subgroupAlias uint64,
	protocols []string,
) *testMoqServer {
	t.Helper()

	cert, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	listener, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   protocols,
	}, &quic.Config{EnableDatagrams: true})
	require.NoError(t, err)

	ctx, ctxCancel := context.WithCancel(context.Background())
	s := &testMoqServer{
		address:       listener.Addr().String(),
		trackAlias:    trackAlias,
		subgroupAlias: subgroupAlias,
		ctx:           ctx,
		ctxCancel:     ctxCancel,
		listener:      listener,
		err:           make(chan error, 1),
		publish:       make(chan publishRequest, 1),
	}

	go s.run()

	return s
}

func newTestMoqServer(t *testing.T, trackAlias uint64, subgroupAlias uint64) *testMoqServer {
	return newTestMoqServerWithProtocols(t, trackAlias, subgroupAlias, []string{testVersion})
}

func (s *testMoqServer) run() {
	conn, err := s.listener.Accept(s.ctx)
	if err != nil {
		if s.ctx.Err() == nil {
			s.fail(err)
		}
		return
	}
	defer conn.CloseWithError(0, "") //nolint:errcheck

	err = s.performSetup(conn)
	if err != nil {
		s.fail(err)
		return
	}

	bidi, err := conn.AcceptStream(s.ctx)
	if err != nil {
		if s.ctx.Err() == nil {
			s.fail(err)
		}
		return
	}
	defer bidi.Close() //nolint:errcheck

	msg, err := controlmessage.Read(bidi)
	if err != nil {
		s.fail(err)
		return
	}

	switch msg := msg.(type) {
	case *controlmessage.Subscribe:
		s.handleSubscribe(conn, bidi, msg)

	case *controlmessage.Publish:
		s.handlePublish(conn, bidi, msg)

	default:
		s.fail(fmt.Errorf("unexpected control message: %T", msg))
	}
}

func (s *testMoqServer) handleSubscribe(conn *quic.Conn, bidi *quic.Stream, sub *controlmessage.Subscribe) {
	if sub.TrackName != "test" {
		s.fail(fmt.Errorf("unexpected track name: %s", sub.TrackName))
		return
	}

	_, err := bidi.Write(controlmessage.SubscribeOk{TrackAlias: s.trackAlias}.Marshal())
	if err != nil {
		s.fail(err)
		return
	}
	bidi.Close() //nolint:errcheck

	uni, err := conn.OpenUniStreamSync(s.ctx)
	if err != nil {
		s.fail(err)
		return
	}
	defer uni.Close() //nolint:errcheck

	sg := subgroup.SubGroup{
		Header: subgroup.Header{
			IsFirstObject: true,
			TrackAlias:    s.subgroupAlias,
		},
		Objects: []subgroup.Object{{Payload: []byte("payload")}},
	}
	_, err = uni.Write(sg.Marshal())
	if err != nil {
		s.fail(err)
		return
	}

	_, _ = io.Copy(io.Discard, bidi)
}

func (s *testMoqServer) handlePublish(conn *quic.Conn, bidi *quic.Stream, pub *controlmessage.Publish) {
	_, err := bidi.Write(controlmessage.RequestOk{}.Marshal())
	if err != nil {
		s.fail(err)
		return
	}
	bidi.Close() //nolint:errcheck

	uni, err := conn.AcceptUniStream(s.ctx)
	if err != nil {
		s.fail(err)
		return
	}

	var sg subgroup.SubGroup
	err = sg.Read(uni)
	if err != nil {
		s.fail(err)
		return
	}

	select {
	case s.publish <- publishRequest{message: *pub, subGroup: sg}:
	case <-s.ctx.Done():
	}

	_, _ = io.Copy(io.Discard, bidi)
}

func (s *testMoqServer) performSetup(conn *quic.Conn) error {
	setupWriter, err := conn.OpenUniStreamSync(s.ctx)
	if err != nil {
		return err
	}

	_, err = setupWriter.Write(controlmessage.Setup{}.Marshal())
	setupWriter.Close() //nolint:errcheck
	if err != nil {
		return err
	}

	setupReader, err := conn.AcceptUniStream(s.ctx)
	if err != nil {
		return err
	}

	msg, err := controlmessage.Read(setupReader)
	if err != nil {
		return err
	}

	setup, ok := msg.(*controlmessage.Setup)
	if !ok {
		return fmt.Errorf("unexpected setup message: %T", msg)
	}
	if setup.Path != "/test" {
		return fmt.Errorf("unexpected setup path: %s", setup.Path)
	}

	return nil
}

func (s *testMoqServer) fail(err error) {
	select {
	case s.err <- err:
	default:
	}
}

func (s *testMoqServer) Close() {
	s.ctxCancel()
	s.listener.Close() //nolint:errcheck
}

func (s *testMoqServer) Check(t *testing.T) {
	t.Helper()

	select {
	case err := <-s.err:
		if !strings.Contains(err.Error(), "Application error 0x0") {
			require.NoError(t, err)
		}
	default:
	}
}

func newClient(t *testing.T, server *testMoqServer) *moq.Client {
	t.Helper()

	u, err := url.Parse("moqt://" + server.address + "/test")
	require.NoError(t, err)

	client := &moq.Client{
		URL: u,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		},
	}
	require.NoError(t, client.Initialize(context.Background()))

	return client
}

func TestClientProtocols(t *testing.T) {
	server := newTestMoqServerWithProtocols(t, 7, 7, []string{string(defs.APIMoQVersionDraft18)})
	defer func() {
		server.Close()
		server.Check(t)
	}()

	u, err := url.Parse("moqt://" + server.address + "/test")
	require.NoError(t, err)

	client := &moq.Client{
		URL:             u,
		TLSConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		ClientProtocols: []string{string(defs.APIMoQVersionDraft18)},
	}
	err = client.Initialize(context.Background())
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck

	require.Equal(t, defs.APIMoQVersionDraft18, client.Version())

	received := make(chan *subgroup.SubGroup, 1)
	err = client.Subscribe(context.Background(), "test", func(sg *subgroup.SubGroup) error {
		received <- sg
		return nil
	})
	require.NoError(t, err)

	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subgroup")
	}
}

func TestClientSubscribe(t *testing.T) {
	server := newTestMoqServer(t, 7, 7)
	defer func() {
		server.Close()
		server.Check(t)
	}()

	client := newClient(t, server)
	defer client.Close() //nolint:errcheck

	received := make(chan *subgroup.SubGroup, 1)
	err := client.Subscribe(context.Background(), "test", func(sg *subgroup.SubGroup) error {
		received <- sg
		return nil
	})
	require.NoError(t, err)

	select {
	case sg := <-received:
		require.Equal(t, uint64(7), sg.Header.TrackAlias)
		require.Equal(t, []byte("payload"), sg.Objects[0].Payload)

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subgroup")
	}
}

func TestClientSubscribeUnknownTrackAlias(t *testing.T) {
	server := newTestMoqServer(t, 7, 8)
	defer func() {
		server.Close()
		server.Check(t)
	}()

	client := newClient(t, server)
	defer client.Close() //nolint:errcheck

	err := client.Subscribe(context.Background(), "test", func(*subgroup.SubGroup) error {
		return nil
	})
	require.NoError(t, err)

	select {
	case err = <-client.Errors():
		require.EqualError(t, err, "received subgroup with unknown track alias: 8")

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for client error")
	}
}

func TestClientPublish(t *testing.T) {
	server := newTestMoqServer(t, 0, 0)
	defer func() {
		server.Close()
		server.Check(t)
	}()

	client := newClient(t, server)
	defer client.Close() //nolint:errcheck

	ts := property.Timestamp(123)
	err := client.Publish(context.Background(), "test", 7, property.Properties{&ts})
	require.NoError(t, err)

	err = client.WriteSubGroup(
		context.Background(),
		7,
		8,
		property.Properties{&ts},
		[]byte("payload"),
	)
	require.NoError(t, err)

	select {
	case req := <-server.publish:
		require.Equal(t, uint64(1), req.message.RequestID)
		require.Equal(t, "test", req.message.TrackName)
		require.Equal(t, uint64(7), req.message.TrackAlias)
		require.Equal(t, property.Properties{&ts}, req.message.TrackProperties)
		require.True(t, req.subGroup.Header.HasProperties)
		require.True(t, req.subGroup.Header.IsFirstObject)
		require.Equal(t, uint64(7), req.subGroup.Header.TrackAlias)
		require.Equal(t, uint64(8), req.subGroup.Header.GroupID)
		require.Equal(t, property.Properties{&ts}, req.subGroup.Objects[0].Properties)
		require.Equal(t, []byte("payload"), req.subGroup.Objects[0].Payload)

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for published subgroup")
	}
}
