package moq_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	mediacommonh264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp3"
	protomoq "github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	ssmoq "github.com/bluenviron/mediamtx/internal/staticsources/moq"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const testVersion = defs.APIMoQVersionDraft19

type testMoqServer struct {
	address            string
	expectedRequestURI string
	expectedAuth       string
	fingerprint        string
	transport          conf.MoQTransport

	ctx       context.Context
	ctxCancel func()
	closeFunc func()
	err       chan error
}

func newTestMoqServer(
	t *testing.T,
	transport conf.MoQTransport,
	expectedRequestURI string,
	expectedAuth string,
) *testMoqServer {
	t.Helper()

	ctx, ctxCancel := context.WithCancel(context.Background())
	ts := &testMoqServer{
		address:            "",
		expectedRequestURI: expectedRequestURI,
		expectedAuth:       expectedAuth,
		fingerprint:        "",
		transport:          transport,
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		err:                make(chan error, 1),
	}

	switch transport {
	case conf.MoQTransportWebTransport:
		ts.initializeWebTransport(t)

	default:
		ts.initializeQUIC(t)
	}

	return ts
}

func (s *testMoqServer) initializeQUIC(t *testing.T) {
	t.Helper()

	cert, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	ln, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{string(testVersion)},
	}, &quic.Config{EnableDatagrams: true})
	require.NoError(t, err)

	s.address = ln.Addr().String()
	s.fingerprint = fingerprintFromRaw(t, cert.Certificate[0])
	s.closeFunc = func() {
		ln.Close() //nolint:errcheck
	}

	go func() {
		for {
			acceptedConn, acceptErr := ln.Accept(s.ctx)
			if s.ctx.Err() != nil {
				return
			}
			if acceptErr != nil {
				s.fail(acceptErr)
				return
			}

			go s.runSession(&protomoq.ConnQUIC{Conn: acceptedConn}, false)
		}
	}()
}

func (s *testMoqServer) initializeWebTransport(t *testing.T) {
	t.Helper()

	addr := freeUDPAddress(t)

	h3s := &httpp3.Server{
		Address:            addr,
		EnableWebTransport: true,
		Parent:             test.NilLogger,
	}
	h3s.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != s.expectedRequestURI {
			s.fail(fmt.Errorf("unexpected request URI: expected %s, got %s", s.expectedRequestURI, r.URL.RequestURI()))
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		offered := r.Header.Get("WT-Available-Protocols")
		if !strings.Contains(offered, string(testVersion)) {
			s.fail(fmt.Errorf("unexpected WT-Available-Protocols header: %s", offered))
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("WT-Protocol", `"`+string(testVersion)+`"`)

		session, err := h3s.Upgrade(w, r)
		if err != nil {
			if s.ctx.Err() == nil {
				s.fail(err)
			}
			return
		}

		go s.runSession(&protomoq.ConnWebTransport{Session: session}, true)
	})

	err := h3s.Initialize()
	require.NoError(t, err)

	s.address = addr
	s.fingerprint = fingerprintFromRaw(t, h3s.Certificate().Certificate[0])
	s.closeFunc = h3s.Close
}

func (s *testMoqServer) runSession(c protomoq.Conn, webTransport bool) {
	defer c.CloseWithError(0, "")

	err := s.performSetup(c, webTransport)
	if err != nil {
		s.fail(err)
		return
	}

	for {
		bidi, acceptErr := c.AcceptStream(s.ctx)
		if s.ctx.Err() != nil {
			return
		}
		if acceptErr != nil {
			if isExpectedSessionShutdownError(acceptErr) {
				return
			}
			s.fail(acceptErr)
			return
		}

		go s.handleSubscribe(c, bidi)
	}
}

func (s *testMoqServer) performSetup(c protomoq.Conn, webTransport bool) error {
	setupWriter, err := c.OpenUniStreamSync(s.ctx)
	if err != nil {
		return err
	}

	_, err = setupWriter.Write(controlmessage.Setup{}.Marshal())
	setupWriter.Close() //nolint:errcheck
	if err != nil {
		return err
	}

	setupReader, err := c.AcceptUniStream(s.ctx)
	if err != nil {
		if isExpectedSessionShutdownError(err) {
			return nil
		}
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

	if webTransport {
		if setup.Path != "" {
			return fmt.Errorf("unexpected WebTransport setup path: %s", setup.Path)
		}
	} else if setup.Path != s.expectedRequestURI {
		return fmt.Errorf("unexpected QUIC setup path: expected %s, got %s", s.expectedRequestURI, setup.Path)
	}

	return nil
}

func (s *testMoqServer) handleSubscribe(c protomoq.Conn, bidi io.ReadWriteCloser) {
	msg, err := controlmessage.Read(bidi)
	if err != nil {
		if s.ctx.Err() == nil {
			s.fail(err)
		}
		bidi.Close() //nolint:errcheck
		return
	}

	sub, ok := msg.(*controlmessage.Subscribe)
	if !ok {
		s.fail(fmt.Errorf("unexpected control message: %T", msg))
		bidi.Close() //nolint:errcheck
		return
	}

	authErr := s.checkAuthorization(sub.Parameters)
	if authErr != nil {
		s.fail(authErr)
		_, _ = bidi.Write(controlmessage.RequestError{
			Code:   controlmessage.RequestErrorCodeUnauthorized,
			Reason: authErr.Error(),
		}.Marshal())
		bidi.Close() //nolint:errcheck
		return
	}

	var payload []byte
	switch sub.TrackName {
	case ".catalog":
		payload, err = catalogPayload()

	case "0":
		payload, err = mediaPayload()

	default:
		_, _ = bidi.Write(controlmessage.RequestError{
			Code:   controlmessage.RequestErrorCodeDoesNotExist,
			Reason: "unknown track",
		}.Marshal())
		bidi.Close() //nolint:errcheck
		return
	}
	if err != nil {
		s.fail(err)
		bidi.Close() //nolint:errcheck
		return
	}

	_, err = bidi.Write(controlmessage.SubscribeOk{TrackAlias: sub.RequestID}.Marshal())
	if err != nil {
		s.fail(err)
		bidi.Close() //nolint:errcheck
		return
	}

	go io.Copy(io.Discard, bidi) //nolint:errcheck

	err = s.writeSubGroup(c, sub.TrackName, sub.RequestID, payload)
	if err != nil {
		s.fail(err)
	}
}

func (s *testMoqServer) checkAuthorization(params parameter.Parameters) error {
	if s.expectedAuth == "" {
		if len(params) != 0 {
			return fmt.Errorf("unexpected authorization parameter")
		}
		return nil
	}

	if len(params) != 1 {
		return fmt.Errorf("expected exactly one authorization parameter")
	}

	tok, ok := params[0].(*parameter.AuthorizationToken)
	if !ok {
		return fmt.Errorf("unexpected authorization parameter type: %T", params[0])
	}
	if tok.AliasType != parameter.AuthorizationTokenAliasTypeUseValue {
		return fmt.Errorf("unexpected authorization alias type: %d", tok.AliasType)
	}
	if tok.TokenType != 1 {
		return fmt.Errorf("unexpected authorization token type: %d", tok.TokenType)
	}
	if string(tok.TokenValue) != s.expectedAuth {
		return fmt.Errorf("unexpected authorization token: expected %q, got %q", s.expectedAuth, string(tok.TokenValue))
	}

	return nil
}

func (s *testMoqServer) writeSubGroup(c protomoq.Conn, trackName string, trackAlias uint64, payload []byte) error {
	uni, err := c.OpenUniStreamSync(s.ctx)
	if err != nil {
		return err
	}
	defer uni.Close() //nolint:errcheck

	sg := &subgroup.SubGroup{
		Header: subgroup.Header{
			IsFirstObject: true,
			TrackAlias:    trackAlias,
			GroupID:       0,
		},
		Objects: []subgroup.Object{{
			Payload: payload,
		}},
	}

	if trackName != ".catalog" {
		ts := property.Timestamp(0)
		sg.Header.HasProperties = true
		sg.Objects[0].Properties = property.Properties{&ts}
	}

	_, err = uni.Write(sg.Marshal())
	return err
}

func isExpectedSessionShutdownError(err error) bool {
	errstr := err.Error()
	return strings.Contains(errstr, "context canceled") ||
		strings.Contains(errstr, "Application error 0x0") ||
		(strings.Contains(errstr, "H3_DATAGRAM_ERROR") &&
			strings.Contains(errstr, "Application error 0x100"))
}

func (s *testMoqServer) fail(err error) {
	select {
	case s.err <- err:
	default:
	}
}

func (s *testMoqServer) Close() {
	s.ctxCancel()
	if s.closeFunc != nil {
		s.closeFunc()
	}
}

func (s *testMoqServer) Check(t *testing.T) {
	t.Helper()

	select {
	case err := <-s.err:
		require.NoError(t, err)
	default:
	}
}

func basicAuthToken(user string, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func catalogPayload() ([]byte, error) {
	return json.Marshal(catalog.Catalog{
		Version: 1,
		Tracks: []catalog.Track{{
			Name:      "0",
			Packaging: "loc",
			IsLive:    true,
			Codec:     "avc3.640028",
		}},
	})
}

func mediaPayload() ([]byte, error) {
	return mediacommonh264.AVCC{test.FormatH264.SPS, test.FormatH264.PPS, {5, 1}}.Marshal()
}

func fingerprintFromRaw(t *testing.T, raw []byte) string {
	t.Helper()

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func freeUDPAddress(t *testing.T) string {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := pc.LocalAddr().String()
	pc.Close() //nolint:errcheck
	return addr
}

type fingerprintErrorParent struct{}

func (*fingerprintErrorParent) Log(_ logger.Level, _ string, _ ...any) {}

func (*fingerprintErrorParent) SetReady(_ defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes {
	panic("should not happen")
}

func (*fingerprintErrorParent) SetNotReady(_ defs.PathSourceStaticSetNotReadyReq) {}

func TestSource(t *testing.T) {
	for _, ca := range []struct {
		name      string
		transport conf.MoQTransport
		withAuth  bool
		withQuery bool
	}{
		{
			name:      "quic",
			transport: conf.MoQTransportQUIC,
		},
		{
			name:      "quic_auth_query",
			transport: conf.MoQTransportQUIC,
			withAuth:  true,
			withQuery: true,
		},
		{
			name:      "webtransport",
			transport: conf.MoQTransportWebTransport,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			expectedRequestURI := "/teststream"
			if ca.withQuery {
				expectedRequestURI += "?key=value"
			}

			expectedAuth := ""
			if ca.withAuth {
				expectedAuth = basicAuthToken("myuser", "mypass")
			}

			server := newTestMoqServer(t, ca.transport, expectedRequestURI, expectedAuth)
			defer func() {
				server.Close()
				server.Check(t)
			}()

			p := &test.StaticSourceParent{}
			p.Initialize()

			so := &ssmoq.Source{
				ReadTimeout: conf.Duration(10 * time.Second),
				Parent:      p,
			}

			resolved := "moqt://" + server.address + "/teststream"
			if ca.withAuth {
				resolved = "moqt://myuser:mypass@" + server.address + "/teststream"
			}
			if ca.withQuery {
				resolved += "?key=value"
			}

			sourceErr := make(chan error, 1)
			go func() {
				sourceErr <- so.Run(defs.StaticSourceRunParams{
					Context:        ctx,
					ResolvedSource: resolved,
					Conf: &conf.Path{
						MoQTransport:      ca.transport,
						SourceFingerprint: server.fingerprint,
					},
					ReloadConf: make(chan *conf.Path),
				})
			}()

			select {
			case u := <-p.Unit:
				require.Equal(t, unit.PayloadH264{test.FormatH264.SPS, test.FormatH264.PPS, {5, 1}}, u.Payload)

			case err := <-sourceErr:
				require.NoError(t, err)
				return

			case <-ctx.Done():
				t.Fatal("timeout waiting for unit")
			}

			cancel()
			require.NoError(t, <-sourceErr)
			p.Close()
		})
	}
}

func TestSourceWebTransportFingerprintError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := newTestMoqServer(t, conf.MoQTransportWebTransport, "/teststream", "")
	defer func() {
		server.Close()
		server.Check(t)
	}()

	so := &ssmoq.Source{Parent: &fingerprintErrorParent{}}

	err := so.Run(defs.StaticSourceRunParams{
		Context:        ctx,
		ResolvedSource: "moqt://" + server.address + "/teststream",
		Conf: &conf.Path{
			MoQTransport:      conf.MoQTransportWebTransport,
			SourceFingerprint: strings.Repeat("0", 64),
		},
		ReloadConf: make(chan *conf.Path),
	})
	require.ErrorContains(t, err, "source fingerprint does not match")
}

func TestSourceFingerprintError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := newTestMoqServer(t, conf.MoQTransportQUIC, "/teststream", "")
	defer func() {
		server.Close()
		server.Check(t)
	}()

	so := &ssmoq.Source{Parent: &fingerprintErrorParent{}}

	err := so.Run(defs.StaticSourceRunParams{
		Context:        ctx,
		ResolvedSource: "moqt://" + server.address + "/teststream",
		Conf: &conf.Path{
			MoQTransport:      conf.MoQTransportQUIC,
			SourceFingerprint: strings.Repeat("0", 64),
		},
		ReloadConf: make(chan *conf.Path),
	})
	require.ErrorContains(t, err, "source fingerprint does not match")
}
