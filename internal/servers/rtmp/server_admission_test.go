package rtmp_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/servers/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func tryDialFrom(t *testing.T, localIP string) (net.Conn, error) {
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(localIP)}}
	nconn, err := d.Dial("tcp", "127.0.0.1:1939")
	if errors.Is(err, syscall.EADDRNOTAVAIL) {
		t.Skipf("unable to dial from %s: %v", localIP, err)
	}
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { nconn.Close() })
	return nconn, nil
}

func dialFrom(t *testing.T, localIP string) net.Conn {
	nconn, err := tryDialFrom(t, localIP)
	require.NoError(t, err)
	return nconn
}

func requireConnCount(t *testing.T, s *rtmp.Server, n int) {
	require.Eventually(t, func() bool {
		list, err := s.APIConnsList()
		return err == nil && len(list.Items) == n
	}, 2*time.Second, 10*time.Millisecond)
}

// requireRejected checks that a new connection is reset by the server.
// The reset can be reported by connect() itself, since it may arrive before
// the client finishes connecting.
func requireRejected(t *testing.T, localIP string, use func(net.Conn) error) {
	nconn, err := tryDialFrom(t, localIP)
	if err == nil {
		require.NoError(t, nconn.SetDeadline(time.Now().Add(2*time.Second)))
		err = use(nconn)
	}
	require.Error(t, err)
	require.True(t, errors.Is(err, syscall.ECONNRESET), "expected connection reset, got %v", err)
}

func readOne(nconn net.Conn) error {
	_, err := nconn.Read(make([]byte, 1))
	return err
}

func TestServerUnauthenticatedLimit(t *testing.T) {
	s := &rtmp.Server{
		Address:                 "127.0.0.1:1939",
		ReadTimeout:             conf.Duration(10 * time.Second),
		WriteTimeout:            conf.Duration(10 * time.Second),
		MaxUnauthenticatedConns: 2,
		PathManager:             &test.PathManager{},
		Parent:                  test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	c1 := dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 1)
	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 2)

	requireRejected(t, "127.0.0.1", readOne)
	require.Equal(t, uint64(1), s.RejectedConns())
	requireConnCount(t, s, 2)

	// slot is reclaimed on disconnect
	c1.Close()
	requireConnCount(t, s, 1)

	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 2)
	require.Equal(t, uint64(1), s.RejectedConns())
}

func TestServerUnauthenticatedLimitPerIP(t *testing.T) {
	s := &rtmp.Server{
		Address:                      "127.0.0.1:1939",
		ReadTimeout:                  conf.Duration(10 * time.Second),
		WriteTimeout:                 conf.Duration(10 * time.Second),
		MaxUnauthenticatedConnsPerIP: 1,
		PathManager:                  &test.PathManager{},
		Parent:                       test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 1)

	requireRejected(t, "127.0.0.1", readOne)
	require.Equal(t, uint64(1), s.RejectedConns())

	dialFrom(t, "127.0.0.2")
	requireConnCount(t, s, 2)
	require.Equal(t, uint64(1), s.RejectedConns())
}

func TestServerUnauthenticatedLimitTimeout(t *testing.T) {
	s := &rtmp.Server{
		Address:                 "127.0.0.1:1939",
		ReadTimeout:             conf.Duration(500 * time.Millisecond),
		WriteTimeout:            conf.Duration(500 * time.Millisecond),
		MaxUnauthenticatedConns: 1,
		PathManager:             &test.PathManager{},
		Parent:                  test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 1)

	// the idle connection is closed by the read deadline and its slot is reclaimed
	requireConnCount(t, s, 0)

	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 1)
	require.Equal(t, uint64(0), s.RejectedConns())
}

func TestServerUnauthenticatedLimitTLS(t *testing.T) {
	serverCertFpath := test.CreateTempFile(t, test.TLSCertPub)
	serverKeyFpath := test.CreateTempFile(t, test.TLSCertKey)

	s := &rtmp.Server{
		Address:                 "127.0.0.1:1939",
		ReadTimeout:             conf.Duration(10 * time.Second),
		WriteTimeout:            conf.Duration(10 * time.Second),
		Encryption:              true,
		ServerCert:              serverCertFpath,
		ServerKey:               serverKeyFpath,
		MaxUnauthenticatedConns: 1,
		PathManager:             &test.PathManager{},
		Parent:                  test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 1)

	// the excess connection is reset before the TLS handshake
	requireRejected(t, "127.0.0.1", func(nconn net.Conn) error {
		return tls.Client(nconn, &tls.Config{InsecureSkipVerify: true}).Handshake()
	})
	require.Equal(t, uint64(1), s.RejectedConns())
}

func TestServerUnauthenticatedLimitReleasedOnAuth(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}
	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		UDPMaxPayloadSize: 1462,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())

	pathManager := &test.PathManager{
		AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
			return &defs.PathAddReaderRes{
				Path:    &dummyPath{},
				Stream:  strm,
				AuthRes: &auth.Result{},
			}, nil
		},
	}

	s := &rtmp.Server{
		Address:                 "127.0.0.1:1939",
		ReadTimeout:             conf.Duration(10 * time.Second),
		WriteTimeout:            conf.Duration(10 * time.Second),
		MaxUnauthenticatedConns: 1,
		PathManager:             pathManager,
		Parent:                  test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	u, err := url.Parse("rtmp://127.0.0.1:1939/teststream")
	require.NoError(t, err)
	client := &gortmplib.Client{URL: u}
	require.NoError(t, client.Initialize(context.Background()))
	defer client.Close()

	require.Eventually(t, func() bool {
		list, listErr := s.APIConnsList()
		return listErr == nil && len(list.Items) == 1 && list.Items[0].State == defs.APIRTMPConnStateRead
	}, 2*time.Second, 10*time.Millisecond)

	// the authorized reader does not occupy the slot
	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 2)

	// saturation rejects new connections without affecting the authorized reader
	requireRejected(t, "127.0.0.1", readOne)
	require.Equal(t, uint64(1), s.RejectedConns())

	list, err := s.APIConnsList()
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
}

func writeProxyV2Header(t *testing.T, nconn net.Conn, proto proxyproto.AddressFamilyAndProtocol, src, dst net.Addr) {
	t.Helper()
	h := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proto,
		SourceAddr:        src,
		DestinationAddr:   dst,
	}
	_, err := h.WriteTo(nconn)
	require.NoError(t, err)
}

func TestServerUnauthenticatedLimitProxyNonTCPNoPanic(t *testing.T) {
	s := &rtmp.Server{
		Address:      "127.0.0.1:1939",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		TrustedProxies: conf.IPNetworks{
			{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(32, 32)},
		},
		PathManager: &test.PathManager{},
		Parent:      test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	// PROXY v2 UDPv4 produces a *net.UDPAddr; must close cleanly without panicking.
	badConn := dialFrom(t, "127.0.0.1")
	writeProxyV2Header(t, badConn, proxyproto.UDPv4,
		&net.UDPAddr{IP: net.ParseIP("203.0.113.5"), Port: 12345},
		&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1939},
	)
	require.NoError(t, badConn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := badConn.Read(make([]byte, 1))
	require.Error(t, err)

	// Server remains healthy and admits valid TCP PROXY connections.
	goodConn := dialFrom(t, "127.0.0.1")
	writeProxyV2Header(t, goodConn, proxyproto.TCPv4,
		&net.TCPAddr{IP: net.ParseIP("203.0.113.6"), Port: 12346},
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1939},
	)
	requireConnCount(t, s, 1)
}

func TestServerUnauthenticatedLimitProxySlowHeaderAndGlobalCap(t *testing.T) {
	s := &rtmp.Server{
		Address:      "127.0.0.1:1939",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		TrustedProxies: conf.IPNetworks{
			{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(32, 32)},
		},
		MaxUnauthenticatedConns: 1,
		PathManager:             &test.PathManager{},
		Parent:                  test.NilLogger,
	}
	require.NoError(t, s.Initialize())

	// Silent connection never sends a PROXY header; occupies the single global slot.
	dialFrom(t, "127.0.0.1")

	// Server.run() must remain responsive (not stalled waiting for PROXY header).
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, listErr := s.APIConnsList()
		require.NoError(t, listErr)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("APIConnsList blocked on slow PROXY header")
	}

	// Global cap rejects the second connection immediately before any PROXY header read.
	requireRejected(t, "127.0.0.1", readOne)
	require.Equal(t, uint64(1), s.RejectedConns())

	// Close must cancel the in-flight PROXY header read promptly.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		s.Close()
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Server.Close blocked on slow PROXY header")
	}
}

func TestServerUnauthenticatedLimitProxyPerIP(t *testing.T) {
	s := &rtmp.Server{
		Address:      "127.0.0.1:1939",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		TrustedProxies: conf.IPNetworks{
			{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(32, 32)},
		},
		MaxUnauthenticatedConnsPerIP: 1,
		PathManager:                  &test.PathManager{},
		Parent:                       test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	c1 := dialFrom(t, "127.0.0.1")
	writeProxyV2Header(t, c1, proxyproto.TCPv4,
		&net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 10001},
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1939},
	)
	requireConnCount(t, s, 1)

	// Same PROXY-supplied client IP is rejected with RST.
	requireRejected(t, "127.0.0.1", func(nconn net.Conn) error {
		writeProxyV2Header(t, nconn, proxyproto.TCPv4,
			&net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 10002},
			&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1939},
		)
		return readOne(nconn)
	})
	require.Equal(t, uint64(1), s.RejectedConns())

	// Different PROXY-supplied client IP is admitted.
	c3 := dialFrom(t, "127.0.0.1")
	writeProxyV2Header(t, c3, proxyproto.TCPv4,
		&net.TCPAddr{IP: net.ParseIP("203.0.113.11"), Port: 10003},
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1939},
	)
	requireConnCount(t, s, 2)
	require.Equal(t, uint64(1), s.RejectedConns())
}

func TestServerUnauthenticatedLimitReleasedBeforePathReady(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}
	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		UDPMaxPayloadSize: 1462,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())

	authorizedCh := make(chan struct{})
	unblockReader := make(chan struct{})
	pathManager := &test.PathManager{
		AddReaderImpl: func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
			if req.OnAuthorized != nil {
				req.OnAuthorized()
			}
			close(authorizedCh)
			<-unblockReader
			return &defs.PathAddReaderRes{
				Path:    &dummyPath{},
				Stream:  strm,
				AuthRes: &auth.Result{},
			}, nil
		},
	}

	s := &rtmp.Server{
		Address:                 "127.0.0.1:1939",
		ReadTimeout:             conf.Duration(10 * time.Second),
		WriteTimeout:            conf.Duration(10 * time.Second),
		MaxUnauthenticatedConns: 1,
		PathManager:             pathManager,
		Parent:                  test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	u, err := url.Parse("rtmp://127.0.0.1:1939/ondemand")
	require.NoError(t, err)
	client := &gortmplib.Client{URL: u}
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- client.Initialize(context.Background())
	}()
	defer func() {
		select {
		case <-unblockReader:
		default:
			close(unblockReader)
		}
		client.Close()
	}()

	<-authorizedCh

	// While the authorized reader is still waiting for the on-demand path, a second connection is admitted.
	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 2)
	require.Equal(t, uint64(0), s.RejectedConns())

	close(unblockReader)
	require.NoError(t, <-clientDone)
}

func TestServerUnauthenticatedLimitReleasedOnPublishAuth(t *testing.T) {
	pathManager := &test.PathManager{
		FindPathConfImpl: func(_ defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
			return &defs.PathFindPathConfRes{
				Conf:    &conf.Path{},
				AuthRes: &auth.Result{},
			}, nil
		},
		AddPublisherImpl: func(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
			strm := &stream.Stream{
				OrigDesc:          req.Desc,
				WriteQueueSize:    512,
				UDPMaxPayloadSize: 1462,
				Parent:            test.NilLogger,
			}
			require.NoError(t, strm.Initialize())
			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			require.NoError(t, subStream.Initialize())
			return &defs.PathAddPublisherRes{
				Path:      &dummyPath{},
				SubStream: subStream,
				AuthRes:   &auth.Result{},
			}, nil
		},
	}

	s := &rtmp.Server{
		Address:                 "127.0.0.1:1939",
		ReadTimeout:             conf.Duration(10 * time.Second),
		WriteTimeout:            conf.Duration(10 * time.Second),
		MaxUnauthenticatedConns: 1,
		PathManager:             pathManager,
		Parent:                  test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	u, err := url.Parse("rtmp://127.0.0.1:1939/teststream")
	require.NoError(t, err)
	client := &gortmplib.Client{URL: u, Publish: true}
	require.NoError(t, client.Initialize(context.Background()))
	defer client.Close()

	// After publish auth succeeds, the slot is released before media even starts.
	dialFrom(t, "127.0.0.1")
	requireConnCount(t, s, 2)
	require.Equal(t, uint64(0), s.RejectedConns())
}
