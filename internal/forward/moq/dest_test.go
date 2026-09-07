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

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	mediacommonh264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	forwardmoq "github.com/bluenviron/mediamtx/internal/forward/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp3"
	protomoq "github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const testVersion = "moqt-19"

func isSubGroupStream(b byte) bool {
	return (b & 0x90) == 0x10
}

type receivedTrack struct {
	alias    uint64
	payload  []byte
	hasTS    bool
	ptsValue int64
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
	received  chan receivedTrack
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
		received:           make(chan receivedTrack, 16),
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
		NextProtos:   []string{testVersion},
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
		if !strings.Contains(offered, testVersion) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("WT-Protocol", `"`+testVersion+`"`)

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
				return s.handleBidiStream(bidi)
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

func (s *testMoqServer) handleBidiStream(bidi io.ReadWriteCloser) error {
	msg, err := controlmessage.Read(bidi)
	if err != nil {
		return err
	}

	if m, ok := msg.(*controlmessage.Publish); ok {
		err = s.checkAuthorization(m.Parameters)
		if err != nil {
			return err
		}

		_, err = bidi.Write(controlmessage.RequestOk{}.Marshal())
		if err != nil {
			return err
		}

		bidi.Close()              //nolint:errcheck
		io.Copy(io.Discard, bidi) //nolint:errcheck
		return fmt.Errorf("unexpected publish close")
	}

	return fmt.Errorf("unexpected control message: %T", msg)
}

func (s *testMoqServer) handleUniStream(uni io.Reader) error {
	br := bufio.NewReader(uni)
	firstByte, err := br.Peek(1)
	if err != nil {
		return err
	}

	if isSubGroupStream(firstByte[0]) {
		var sg subgroup.SubGroup
		err = sg.Read(br)
		if err != nil {
			return err
		}

		if len(sg.Objects) == 0 {
			return fmt.Errorf("subgroup has no objects")
		}

		hasTS := false
		var ptsValue int64
		for _, pr := range sg.Objects[0].Properties {
			if ts, ok := pr.(*property.Timestamp); ok {
				hasTS = true
				ptsValue = int64(*ts)
			}
		}

		select {
		case s.received <- receivedTrack{
			alias:    sg.Header.TrackAlias,
			payload:  sg.Objects[0].Payload,
			hasTS:    hasTS,
			ptsValue: ptsValue,
		}:
		default:
		}
	} else {
		var msg controlmessage.Message
		msg, err = controlmessage.Read(br)
		if err != nil {
			return err
		}

		_, ok := msg.(*controlmessage.Setup)
		if !ok {
			return fmt.Errorf("unexpected control message on uni stream: %T", msg)
		}
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

func (s *testMoqServer) Close() {
	s.ctxCancel()
	if s.closeFunc != nil {
		s.closeFunc()
	}
}

func basicAuthToken(user string, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
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

func TestDest(t *testing.T) {
	for _, ca := range []struct {
		name      string
		transport conf.MoQTransport
		withAuth  bool
		withQuery bool
	}{
		{name: "quic", transport: conf.MoQTransportQUIC},
		{name: "quic auth", transport: conf.MoQTransportQUIC, withAuth: true},
		{name: "webtransport", transport: conf.MoQTransportWebTransport},
		{name: "webtransport auth query", transport: conf.MoQTransportWebTransport, withAuth: true, withQuery: true},
	} {
		t.Run(ca.name, func(t *testing.T) {
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

			subStream := &stream.SubStream{Stream: strm, UseRTPPackets: false}
			require.NoError(t, subStream.Initialize())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			resolved := "moqt://" + server.address + "/teststream"
			if ca.withAuth {
				resolved = "moqt://myuser:mypass@" + server.address + "/teststream"
			}
			if ca.withQuery {
				resolved += "?key=value"
			}

			dest := &forwardmoq.Dest{
				Stream:          strm,
				Dest:            resolved,
				DestFingerprint: server.fingerprint,
				Transport:       ca.transport,
				Parent:          test.NilLogger,
			}

			done := make(chan error, 1)
			go func() {
				done <- dest.Run(ctx)
			}()

			strm.WaitForReaders()
			subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadH264{test.FormatH264.SPS, test.FormatH264.PPS, {5, 1}},
			})

			gotCatalog := false
			gotMedia := false
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			for !gotCatalog || !gotMedia {
				select {
				case rec := <-server.received:
					switch rec.alias {
					case 0:
						var cat catalog.Catalog
						require.NoError(t, json.Unmarshal(rec.payload, &cat))
						require.Len(t, cat.Tracks, 1)
						gotCatalog = true
					case 1:
						expected, err := mediacommonh264.AVCC{test.FormatH264.SPS, test.FormatH264.PPS, {5, 1}}.Marshal()
						require.NoError(t, err)
						require.Equal(t, expected, rec.payload)
						require.True(t, rec.hasTS)
						require.Equal(t, int64(0), rec.ptsValue)
						gotMedia = true
					}
				case runErr := <-done:
					t.Fatalf("MoQ destination stopped before forwarding: %v", runErr)
				case <-timer.C:
					t.Fatal("timed out waiting for MoQ forwarded data")
				}
			}

			require.Eventually(t, func() bool {
				return dest.Info().OutboundBytes > 0
			}, 5*time.Second, 10*time.Millisecond)
			require.Eventually(t, func() bool {
				info := dest.Info()
				typeSpecific, ok := info.TypeSpecific.(*defs.APIForwardDestTypeSpecificMoQ)
				return ok && typeSpecific.RemoteAddr != "" &&
					typeSpecific.Transport == string(ca.transport) &&
					typeSpecific.OutboundBytes == info.OutboundBytes
			}, 5*time.Second, 10*time.Millisecond)

			cancel()
			select {
			case runErr := <-done:
				require.Error(t, runErr)
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for MoQ destination to stop")
			}
		})
	}
}
