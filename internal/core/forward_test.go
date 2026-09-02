package core

import (
	"bufio"
	"bytes"
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
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	rtmpcodecs "github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v4"
	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp3"
	"github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	mtxwebrtc "github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/test"
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
		expectedRequestURI: expectedRequestURI,
		expectedAuth:       expectedAuth,
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

			go s.runSession(&moq.ConnQUIC{Conn: acceptedConn})
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

		go s.runSession(&moq.ConnWebTransport{Session: session})
	})

	err := h3s.Initialize()
	require.NoError(t, err)

	s.address = addr
	s.fingerprint = fingerprintFromRaw(t, h3s.Certificate().Certificate[0])
	s.closeFunc = h3s.Close
}

func (s *testMoqServer) runSession(c moq.Conn) {
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

func startRTMPForwardServer(t *testing.T) (string, <-chan [][]byte, <-chan error) {
	ready := &atomic.Bool{}
	ready.Store(true)
	u, received, _, serverErr := startRTMPForwardServerControlled(t, ready)
	return u, received, serverErr
}

func startRTMPForwardServerControlled(
	t *testing.T,
	ready *atomic.Bool,
) (string, <-chan [][]byte, <-chan struct{}, <-chan error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		ln.Close()
	})

	received := make(chan [][]byte, 16)
	connOpened := make(chan struct{}, 16)
	serverErr := make(chan error, 16)

	go func() {
		for {
			nconn, acceptErr := ln.Accept()
			if acceptErr != nil {
				select {
				case <-done:
				default:
					serverErr <- acceptErr
				}
				return
			}

			if !ready.Load() {
				nconn.Close()
				continue
			}

			select {
			case connOpened <- struct{}{}:
			default:
			}

			go handleRTMPForwardConn(nconn, received, serverErr)
		}
	}()

	return "rtmp://" + ln.Addr().String() + "/dest", received, connOpened, serverErr
}

func handleRTMPForwardConn(nconn net.Conn, received chan<- [][]byte, serverErr chan<- error) {
	defer nconn.Close()

	deadlineErr := nconn.SetDeadline(time.Now().Add(10 * time.Second))
	if deadlineErr != nil {
		serverErr <- deadlineErr
		return
	}

	conn := &gortmplib.ServerConn{RW: nconn}
	initErr := conn.Initialize()
	if initErr != nil {
		serverErr <- initErr
		return
	}

	acceptConnErr := conn.AcceptConn()
	if acceptConnErr != nil {
		serverErr <- acceptConnErr
		return
	}

	if !conn.Publish {
		serverErr <- fmt.Errorf("connection is not publishing")
		return
	}
	if conn.URL.Path != "/dest" {
		serverErr <- fmt.Errorf("unexpected path: %s", conn.URL.Path)
		return
	}

	r := &gortmplib.Reader{Conn: conn}
	err := r.Initialize()
	if err != nil {
		serverErr <- err
		return
	}

	tracks := r.Tracks()
	if len(tracks) != 1 {
		serverErr <- fmt.Errorf("unexpected track count: %d", len(tracks))
		return
	}
	if _, ok := tracks[0].Codec.(*rtmpcodecs.H264); !ok {
		serverErr <- fmt.Errorf("unexpected codec: %T", tracks[0].Codec)
		return
	}

	r.OnDataH264(tracks[0], func(_ time.Duration, _ time.Duration, au [][]byte) {
		for _, nalu := range au {
			if bytes.Equal(nalu, []byte{5, 2, 3, 4}) {
				select {
				case received <- au:
				default:
				}
			}
		}
	})

	for {
		err = r.Read()
		if err != nil {
			return
		}
	}
}

func startRTMPPublisher(
	t *testing.T,
	path string,
) (*gortmplib.Client, *gortmplib.Writer, *gortmplib.Track) {
	u, err := url.Parse("rtmp://127.0.0.1:1935/" + path)
	require.NoError(t, err)

	source := &gortmplib.Client{
		URL:     u,
		Publish: true,
	}
	err = source.Initialize(context.Background())
	require.NoError(t, err)

	track := &gortmplib.Track{
		Codec: &rtmpcodecs.H264{
			SPS: test.FormatH264.SPS,
			PPS: test.FormatH264.PPS,
		},
	}

	w := &gortmplib.Writer{
		Conn:   source,
		Tracks: []*gortmplib.Track{track},
	}
	err = w.Initialize()
	require.NoError(t, err)

	return source, w, track
}

func waitRTMPForwardFrame(
	t *testing.T,
	w *gortmplib.Writer,
	track *gortmplib.Track,
	received <-chan [][]byte,
	serverErr <-chan error,
) {
	t.Helper()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	for {
		select {
		case au := <-received:
			require.Contains(t, au, []byte{5, 2, 3, 4})
			return

		case err := <-serverErr:
			require.NoError(t, err)

		case <-ticker.C:
			err := w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
			require.NoError(t, err)

		case <-timer.C:
			t.Fatal("timed out waiting for RTMP forwarded frame")
		}
	}
}

func startWHIPForwardServer(
	t *testing.T,
	expectedBearerToken string,
) (string, <-chan struct{}, <-chan error) {
	pc := &mtxwebrtc.PeerConnection{
		LocalRandomUDP:    true,
		IPsFromInterfaces: true,
		Log:               test.NilLogger,
	}
	err := pc.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		pc.Close()
	})

	received := make(chan struct{}, 16)
	serverErr := make(chan error, 16)

	httpServ := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expectedBearerToken != "" {
				require.Equal(t, "Bearer "+expectedBearerToken, r.Header.Get("Authorization"))
			}

			switch {
			case r.Method == http.MethodOptions && r.URL.Path == "/teststream/whip":
				w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
				w.WriteHeader(http.StatusNoContent)

			case r.Method == http.MethodPost && r.URL.Path == "/teststream/whip":
				require.Equal(t, "application/sdp", r.Header.Get("Content-Type"))

				body, err2 := io.ReadAll(r.Body)
				require.NoError(t, err2)
				offer := &pwebrtc.SessionDescription{
					Type: pwebrtc.SDPTypeOffer,
					SDP:  string(body),
				}

				answer, err2 := pc.CreateFullAnswer(offer, false)
				require.NoError(t, err2)

				w.Header().Set("Content-Type", "application/sdp")
				w.Header().Set("ETag", "test_etag")
				w.Header().Set("Location", "/teststream/whip/sessionid")
				w.WriteHeader(http.StatusCreated)
				_, err2 = w.Write([]byte(answer.SDP))
				require.NoError(t, err2)

				go func() {
					err3 := pc.WaitUntilConnected(10 * time.Second)
					if err3 != nil {
						serverErr <- err3
						return
					}

					err3 = pc.GatherInboundTracks(2 * time.Second)
					if err3 != nil {
						serverErr <- err3
						return
					}

					if len(pc.InboundTracks()) != 1 {
						serverErr <- fmt.Errorf("unexpected track count: %d", len(pc.InboundTracks()))
						return
					}

					pc.InboundTracks()[0].OnPacketRTP = func(_ *rtp.Packet) {
						select {
						case received <- struct{}{}:
						default:
						}
					}

					pc.StartReading()
				}()

			case r.URL.Path == "/teststream/whip/sessionid" && r.Method == http.MethodPatch:
				w.WriteHeader(http.StatusNoContent)

			case r.URL.Path == "/teststream/whip/sessionid" && r.Method == http.MethodDelete:
				w.WriteHeader(http.StatusOK)

			default:
				serverErr <- fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusBadRequest)
			}
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go httpServ.Serve(ln)

	t.Cleanup(func() {
		httpServ.Shutdown(context.Background())
	})

	return "whip://" + ln.Addr().String() + "/teststream/whip", received, serverErr
}

func waitWHIPForwardFrame(
	t *testing.T,
	w *gortmplib.Writer,
	track *gortmplib.Track,
	received <-chan struct{},
	serverErr <-chan error,
) {
	t.Helper()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-received:
			return

		case err := <-serverErr:
			require.NoError(t, err)

		case <-ticker.C:
			err := w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
			require.NoError(t, err)

		case <-timer.C:
			t.Fatal("timed out waiting for WHIP forwarded frame")
		}
	}
}

func TestPathForwardRTMP(t *testing.T) {
	dest, received, serverErr := startRTMPForwardServer(t)

	p, ok := newInstance(t, "api: yes\n"+
		"moq: no\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+dest+"\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source, w, track := startRTMPPublisher(t, "source")
	defer source.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	err := w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var path struct {
			Ready bool `json:"ready"`
		}
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/source", nil, &path)
		return path.Ready
	}, 5*time.Second, 100*time.Millisecond)

	var list defs.APIForwardDestList
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
	require.Len(t, list.Items, 1)
	added := list.Items[0]
	require.Equal(t, dest, added.Conf.Dest)
	require.Equal(t, defs.APIForwardDestTypeRTMP, added.Type)
	require.Equal(t, defs.APIForwardDestProtocolRTMP, added.Protocol)
	require.Equal(t, 1, added.Pos)

	waitRTMPForwardFrame(t, w, track, received, serverErr)

	require.Eventually(t, func() bool {
		var item defs.APIForwardDest
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/get?path=source&id="+added.ID.String(), nil, &item)
		return item.State == defs.APIForwardDestStateForwarding &&
			item.Type == defs.APIForwardDestTypeRTMP &&
			item.Protocol == defs.APIForwardDestProtocolRTMP &&
			item.OutboundBytes > 0
	}, 5*time.Second, 100*time.Millisecond)
}

func TestPathForwardRTMPReconnectsAfterSourceUnavailable(t *testing.T) {
	dest, received, serverErr := startRTMPForwardServer(t)

	p, ok := newInstance(t, "api: yes\n"+
		"moq: no\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+dest+"\n")
	require.Equal(t, true, ok)
	defer p.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var id uuid.UUID
	require.Eventually(t, func() bool {
		var list defs.APIForwardDestList
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
		if list.ItemCount != 1 || list.Items[0].State != defs.APIForwardDestStateIdle {
			return false
		}
		id = list.Items[0].ID
		return true
	}, 7*time.Second, 100*time.Millisecond)

	source, w, track := startRTMPPublisher(t, "source")
	waitRTMPForwardFrame(t, w, track, received, serverErr)
	source.Close()

	require.Eventually(t, func() bool {
		var list defs.APIForwardDestList
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
		return list.ItemCount == 1 && list.Items[0].ID == id
	}, 5*time.Second, 100*time.Millisecond)

	for {
		select {
		case <-received:
		default:
			goto drained
		}
	}

drained:
	source, w, track = startRTMPPublisher(t, "source")
	defer source.Close()

	waitRTMPForwardFrame(t, w, track, received, serverErr)

	var item defs.APIForwardDest
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/get?path=source&id="+id.String(), nil, &item)
	require.Equal(t, id, item.ID)
	require.Equal(t, defs.APIForwardDestStateForwarding, item.State)
	require.Greater(t, item.OutboundBytes, uint64(0))
}

func TestPathForwardRTMPReconnectsAfterDestinationUnavailable(t *testing.T) {
	ready := &atomic.Bool{}
	dest, received, _, serverErr := startRTMPForwardServerControlled(t, ready)

	p, ok := newInstance(t, "api: yes\n"+
		"moq: no\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+dest+"\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source, w, track := startRTMPPublisher(t, "source")
	defer source.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	var id uuid.UUID
	require.Eventually(t, func() bool {
		var list defs.APIForwardDestList
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
		if list.ItemCount != 1 || list.Items[0].State != defs.APIForwardDestStateIdle {
			return false
		}
		id = list.Items[0].ID
		return true
	}, 7*time.Second, 100*time.Millisecond)

	ready.Store(true)
	waitRTMPForwardFrame(t, w, track, received, serverErr)

	var item defs.APIForwardDest
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/get?path=source&id="+id.String(), nil, &item)
	require.Equal(t, id, item.ID)
	require.Equal(t, defs.APIForwardDestStateForwarding, item.State)
	require.Equal(t, defs.APIForwardDestTypeRTMP, item.Type)
	require.Equal(t, defs.APIForwardDestProtocolRTMP, item.Protocol)
	require.Greater(t, item.OutboundBytes, uint64(0))
}

func TestPathForwardMoQ(t *testing.T) {
	server := newTestMoqServer(t,
		conf.MoQTransportWebTransport,
		"/teststream?key=value",
		basicAuthToken("myuser", "mypass"),
	)

	p, ok := newInstance(t, "api: yes\n"+
		"moq: no\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: moqt://myuser:mypass@"+server.address+"/teststream?key=value\n"+
		"      destFingerprint: "+server.fingerprint+"\n"+
		"      moqTransport: webtransport\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source, w, track := startRTMPPublisher(t, "source")
	defer source.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	err := w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var path struct {
			Ready bool `json:"ready"`
		}
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/source", nil, &path)
		return path.Ready
	}, 5*time.Second, 100*time.Millisecond)

	var list defs.APIForwardDestList
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
	require.Len(t, list.Items, 1)
	added := list.Items[0]
	require.Equal(t, defs.APIForwardDestProtocolMoQ, added.Protocol)
	require.Equal(t, conf.MoQTransportWebTransport, added.Conf.MoQTransport)
	require.Equal(t, 1, added.Pos)

	gotCatalog := false
	gotMedia := false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for gotCatalog || gotMedia {
		select {
		case rec := <-server.received:
			switch rec.alias {
			case 0:
				var cat catalog.Catalog
				require.NoError(t, json.Unmarshal(rec.payload, &cat))
				require.Len(t, cat.Tracks, 1)
				gotCatalog = true

			case 1:
				require.True(t, rec.hasTS)
				require.NotEmpty(t, rec.payload)
				require.True(t, bytes.Contains(rec.payload, []byte{5, 2, 3, 4}))
				gotMedia = true
			}

		case <-ticker.C:
			err = w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
			require.NoError(t, err)

		case <-timer.C:
			t.Fatal("timed out waiting for MoQ forwarded data")
		}
	}

	require.Eventually(t, func() bool {
		var item defs.APIForwardDest
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/get?path=source&id="+added.ID.String(), nil, &item)
		return item.State == defs.APIForwardDestStateForwarding &&
			item.Type == defs.APIForwardDestTypeMoQ &&
			item.Protocol == defs.APIForwardDestProtocolMoQ &&
			item.Conf.MoQTransport == conf.MoQTransportWebTransport &&
			item.OutboundBytes > 0
	}, 10*time.Second, 200*time.Millisecond)

	server.Close()
}

func TestPathForwardWHIP(t *testing.T) {
	const bearerToken = "mytoken"

	dest, received, serverErr := startWHIPForwardServer(t, bearerToken)

	p, ok := newInstance(t, "api: yes\n"+
		"moq: no\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+dest+"\n"+
		"      whipBearerToken: "+bearerToken+"\n")
	require.Equal(t, true, ok)
	defer p.Close()

	source, w, track := startRTMPPublisher(t, "source")
	defer source.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	err := w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var path struct {
			Ready bool `json:"ready"`
		}
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/source", nil, &path)
		return path.Ready
	}, 5*time.Second, 100*time.Millisecond)

	var list defs.APIForwardDestList
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
	require.Len(t, list.Items, 1)
	added := list.Items[0]
	require.Equal(t, dest, added.Conf.Dest)
	require.Equal(t, bearerToken, added.Conf.WHIPBearerToken)
	require.Equal(t, defs.APIForwardDestTypeWebRTC, added.Type)
	require.Equal(t, defs.APIForwardDestProtocolWHIP, added.Protocol)
	require.Equal(t, 1, added.Pos)

	waitWHIPForwardFrame(t, w, track, received, serverErr)

	require.Eventually(t, func() bool {
		var item defs.APIForwardDest
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/get?path=source&id="+added.ID.String(), nil, &item)
		return item.State == defs.APIForwardDestStateForwarding &&
			item.Type == defs.APIForwardDestTypeWebRTC &&
			item.Protocol == defs.APIForwardDestProtocolWHIP &&
			item.Conf.WHIPBearerToken == bearerToken &&
			item.OutboundBytes > 0
	}, 5*time.Second, 100*time.Millisecond)
}
