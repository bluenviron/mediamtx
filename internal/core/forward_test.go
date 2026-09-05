package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	rtmpcodecs "github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
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
	address      string
	expectedAuth string
	fingerprint  string

	ctx       context.Context
	ctxCancel func()
	closeFunc func()
	received  chan receivedTrack
}

func newTestMoqServer(
	t *testing.T,
	expectedAuth string,
) *testMoqServer {
	t.Helper()

	ctx, ctxCancel := context.WithCancel(context.Background())
	ts := &testMoqServer{
		expectedAuth: expectedAuth,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
		received:     make(chan receivedTrack, 16),
	}

	cert, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	ln, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{testVersion},
	}, &quic.Config{EnableDatagrams: true})
	require.NoError(t, err)

	ts.address = ln.Addr().String()
	ts.fingerprint = fingerprintFromRaw(t, cert.Certificate[0])
	ts.closeFunc = func() {
		ln.Close() //nolint:errcheck
	}

	go func() {
		for {
			acceptedConn, err2 := ln.Accept(ts.ctx)
			if err2 != nil {
				return
			}

			go ts.runSession(&moq.ConnQUIC{Conn: acceptedConn})
		}
	}()

	return ts
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

func TestPathForward(t *testing.T) {
	rtmpDest, rtmpReceived, serverErr := startRTMPForwardServer(t)
	moqServer := newTestMoqServer(t, basicAuthToken("myuser", "mypass"))
	moqDest := "moqt://myuser:mypass@" + moqServer.address + "/teststream?key=value"

	p, ok := newInstance(t, "api: yes\n"+
		"moq: no\n"+
		"paths:\n"+
		"  source:\n"+
		"    forward:\n"+
		"    - dest: "+rtmpDest+"\n"+
		"    - dest: "+moqDest+"\n"+
		"      moqTransport: quic\n"+
		"      destFingerprint: "+moqServer.fingerprint+"\n")
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

	var list map[string]any
	httpRequest(t, hc, http.MethodGet,
		"http://localhost:9997/v3/paths/forward/list?path=source", nil, &list)
	items := list["items"].([]any)
	require.Len(t, items, 2)
	var rtmpItem map[string]any
	var moqItem map[string]any
	for _, rawItem := range items {
		item := rawItem.(map[string]any)
		switch apiMapString(item, "type") {
		case string(defs.APIForwardDestTypeRTMP):
			rtmpItem = item
		case string(defs.APIForwardDestTypeMoQ):
			moqItem = item
		}
	}
	require.NotNil(t, rtmpItem)
	require.NotNil(t, moqItem)
	require.Equal(t, rtmpDest, apiMapString(rtmpItem["conf"].(map[string]any), "dest"))
	require.Equal(t, moqDest, apiMapString(moqItem["conf"].(map[string]any), "dest"))
	require.Equal(t, string(defs.APIForwardDestProtocolRTMP), apiMapString(rtmpItem, "protocol"))
	require.Equal(t, string(defs.APIForwardDestProtocolMoQ), apiMapString(moqItem, "protocol"))
	require.Equal(t, float64(1), apiMapNumber(rtmpItem, "pos"))
	require.Equal(t, float64(2), apiMapNumber(moqItem, "pos"))

	rtmpReceivedFrame := false
	moqReceivedFrame := false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for !rtmpReceivedFrame || !moqReceivedFrame {
		select {
		case au := <-rtmpReceived:
			require.Contains(t, au, []byte{5, 2, 3, 4})
			rtmpReceivedFrame = true
		case received := <-moqServer.received:
			if bytes.Contains(received.payload, []byte{5, 2, 3, 4}) {
				moqReceivedFrame = true
			}
		case err = <-serverErr:
			require.NoError(t, err)
		case <-ticker.C:
			err = w.WriteH264(track, 2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
			require.NoError(t, err)
		case <-timer.C:
			t.Fatal("timed out waiting for MoQ forwarded frame")
		}
	}

	require.Eventually(t, func() bool {
		var item map[string]any
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/get?path=source&id="+apiMapUUID(t, rtmpItem, "id").String(), nil, &item)
		return apiMapString(item, "state") == string(defs.APIForwardDestStateForwarding) &&
			apiMapString(item, "type") == string(defs.APIForwardDestTypeRTMP) &&
			apiMapString(item, "protocol") == string(defs.APIForwardDestProtocolRTMP) &&
			apiMapNumber(item, "outboundBytes") > 0
	}, 5*time.Second, 100*time.Millisecond)

	require.Eventually(t, func() bool {
		var item map[string]any
		httpRequest(t, hc, http.MethodGet,
			"http://localhost:9997/v3/paths/forward/get?path=source&id="+apiMapUUID(t, moqItem, "id").String(), nil, &item)
		return apiMapString(item, "state") == string(defs.APIForwardDestStateForwarding) &&
			apiMapString(item, "type") == string(defs.APIForwardDestTypeMoQ) &&
			apiMapString(item, "protocol") == string(defs.APIForwardDestProtocolMoQ) &&
			apiMapNumber(item, "outboundBytes") > 0
	}, 5*time.Second, 100*time.Millisecond)
}
