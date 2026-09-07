package moq_test

import (
	"bufio"
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
	"golang.org/x/sync/errgroup"

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

func isSubGroupStream(b byte) bool {
	return (b & 0x90) == 0x10
}

type testMoqServer struct {
	address            string
	expectedRequestURI string
	expectedAuth       string
	fingerprint        string
	transport          conf.MoQTransport

	ctx       context.Context
	ctxCancel func()
	closeFunc func()
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
			acceptedConn, err2 := ln.Accept(s.ctx)
			if err2 != nil {
				return
			}

			go s.runSession(&protomoq.ConnQUIC{Conn: acceptedConn})
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
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		offered := r.Header.Get("WT-Available-Protocols")
		if !strings.Contains(offered, string(testVersion)) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("WT-Protocol", `"`+string(testVersion)+`"`)

		session, err := h3s.Upgrade(w, r)
		if err != nil {
			return
		}

		go s.runSession(&protomoq.ConnWebTransport{Session: session})
	})

	err := h3s.Initialize()
	require.NoError(t, err)

	s.address = addr
	s.fingerprint = fingerprintFromRaw(t, h3s.Certificate().Certificate[0])
	s.closeFunc = h3s.Close
}

func (s *testMoqServer) runSession(c protomoq.Conn) {
	defer c.CloseWithError(0, "")

	eg, egCtx := errgroup.WithContext(s.ctx)

	eg.Go(func() error {
		for {
			bidi, err := c.AcceptStream(egCtx)
			if err != nil {
				return err
			}

			eg.Go(func() error {
				return s.handleBidiStream(bidi, c)
			})
		}
	})

	eg.Go(func() error {
		for {
			uni, err := c.AcceptUniStream(egCtx)
			if err != nil {
				return err
			}

			eg.Go(func() error {
				return s.handleUniStream(uni)
			})
		}
	})

	eg.Go(func() error {
		setupWriter, err := c.OpenUniStreamSync(s.ctx)
		if err != nil {
			return err
		}
		defer setupWriter.Close() //nolint:errcheck

		_, err = setupWriter.Write(controlmessage.Setup{}.Marshal())
		return err
	})

	eg.Wait() //nolint:errcheck
}

func (s *testMoqServer) handleBidiStream(bidi io.ReadWriteCloser, c protomoq.Conn) error {
	msg, err := controlmessage.Read(bidi)
	if err != nil {
		return err
	}

	sub, ok := msg.(*controlmessage.Subscribe)
	if !ok {
		return fmt.Errorf("unexpected control message on bidi stream: %T", msg)
	}

	err = s.checkAuthorization(sub.Parameters)
	if err != nil {
		return err
	}

	var payload []byte
	switch sub.TrackName {
	case ".catalog":
		payload, err = catalogPayload()
		if err != nil {
			return err
		}

	case "0":
		payload, err = mediaPayload()
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown track")
	}

	_, err = bidi.Write(controlmessage.SubscribeOk{TrackAlias: sub.RequestID}.Marshal())
	if err != nil {
		return err
	}

	go func() {
		s.writeSubGroup(c, sub.TrackName, sub.RequestID, payload) //nolint:errcheck
	}()

	bidi.Close()              //nolint:errcheck
	io.Copy(io.Discard, bidi) //nolint:errcheck

	return nil
}

func (s *testMoqServer) handleUniStream(uni io.Reader) error {
	br := bufio.NewReader(uni)
	firstByte, err := br.Peek(1)
	if err != nil {
		return err
	}

	if isSubGroupStream(firstByte[0]) {
		return fmt.Errorf("unexpected sub-group stream on uni stream")
	}

	msg, err := controlmessage.Read(br)
	if err != nil {
		return err
	}

	_, ok := msg.(*controlmessage.Setup)
	if !ok {
		return fmt.Errorf("unexpected control message on uni stream: %T", msg)
	}

	return nil
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

func (s *testMoqServer) Close() {
	s.ctxCancel()
	if s.closeFunc != nil {
		s.closeFunc()
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
			defer server.Close()

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

				require.Eventually(t, func() bool {
					info := so.Info()
					typeSpecific, ok := info.TypeSpecific.(*defs.APIStaticSourceTypeSpecificMoQ)
					return ok && typeSpecific.RemoteAddr != "" && typeSpecific.Transport != "" && typeSpecific.InboundBytes > 0
				}, 5*time.Second, 10*time.Millisecond)

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
	defer server.Close()

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
	defer server.Close()

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
