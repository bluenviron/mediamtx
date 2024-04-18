package webrtc

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func uint16Ptr(v uint16) *uint16 {
	return &v
}

func checkClose(t *testing.T, closeFunc func() error) {
	require.NoError(t, closeFunc())
}

type dummyPath struct {
	stream        *stream.Stream
	streamCreated chan struct{}
}

func (p *dummyPath) Name() string {
	return "teststream"
}

func (p *dummyPath) SafeConf() *conf.Path {
	return &conf.Path{}
}

func (p *dummyPath) ExternalCmdEnv() externalcmd.Environment {
	return externalcmd.Environment{}
}

func (p *dummyPath) StartPublisher(req defs.PathStartPublisherReq) (*stream.Stream, error) {
	var err error
	p.stream, err = stream.New(
		1460,
		req.Desc,
		true,
		test.NilLogger,
	)
	if err != nil {
		return nil, err
	}
	close(p.streamCreated)
	return p.stream, nil
}

func (p *dummyPath) StopPublisher(_ defs.PathStopPublisherReq) {
}

func (p *dummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {
}

func (p *dummyPath) RemoveReader(_ defs.PathRemoveReaderReq) {
}

type dummyPathManager struct {
	path *dummyPath
}

func (pm *dummyPathManager) FindPathConf(req defs.PathFindPathConfReq) (*conf.Path, error) {
	if req.AccessRequest.User != "myuser" || req.AccessRequest.Pass != "mypass" {
		return nil, auth.Error{}
	}
	return &conf.Path{}, nil
}

func (pm *dummyPathManager) AddPublisher(_ defs.PathAddPublisherReq) (defs.Path, error) {
	return pm.path, nil
}

func (pm *dummyPathManager) AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
	if req.AccessRequest.Name == "nonexisting" {
		return nil, nil, &defs.PathNoOnePublishingError{}
	}
	return pm.path, pm.path.stream, nil
}

func initializeTestServer(t *testing.T) *Server {
	path := &dummyPath{
		streamCreated: make(chan struct{}),
	}

	pathManager := &dummyPathManager{path: path}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.StringDuration(10 * time.Second),
		WriteQueueSize:        512,
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers:            []conf.WebRTCICEServer{},
		ExternalCmdPool:       nil,
		PathManager:           pathManager,
		Parent:                test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)

	return s
}

func TestServerStaticPages(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	for _, path := range []string{"/stream", "/stream/publish", "/publish"} {
		func() {
			req, err := http.NewRequest(http.MethodGet, "http://myuser:mypass@localhost:8886"+path, nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)
		}()
	}
}

func TestServerOptionsPreflight(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	// preflight requests must always work, without authentication
	req, err := http.NewRequest(http.MethodOptions, "http://localhost:8886/teststream/whip", nil)
	require.NoError(t, err)

	req.Header.Set("Access-Control-Request-Method", "OPTIONS")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	_, ok := res.Header["Link"]
	require.Equal(t, false, ok)
}

func TestServerOptionsICEServer(t *testing.T) {
	pathManager := &dummyPathManager{}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.StringDuration(10 * time.Second),
		WriteQueueSize:        512,
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers: []conf.WebRTCICEServer{{
			URL:      "example.com",
			Username: "myuser",
			Password: "mypass",
		}},
		ExternalCmdPool: nil,
		PathManager:     pathManager,
		Parent:          test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions,
		"http://myuser:mypass@localhost:8886/nonexisting/whep", nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	iceServers, err := webrtc.LinkHeaderUnmarshal(res.Header["Link"])
	require.NoError(t, err)

	require.Equal(t, []pwebrtc.ICEServer{{
		URLs:       []string{"example.com"},
		Username:   "myuser",
		Credential: "mypass",
	}}, iceServers)
}

func TestServerPublish(t *testing.T) {
	path := &dummyPath{
		streamCreated: make(chan struct{}),
	}

	pathManager := &dummyPathManager{path: path}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.StringDuration(10 * time.Second),
		WriteQueueSize:        512,
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers:            []conf.WebRTCICEServer{},
		ExternalCmdPool:       nil,
		PathManager:           pathManager,
		Parent:                test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	su, err := url.Parse("http://myuser:mypass@localhost:8886/teststream/whip?param=value")
	require.NoError(t, err)

	wc := &webrtc.WHIPClient{
		HTTPClient: hc,
		URL:        su,
		Log:        test.NilLogger,
	}

	tracks, err := wc.Publish(context.Background(), test.FormatH264, nil)
	require.NoError(t, err)
	defer checkClose(t, wc.Close)

	err = tracks[0].WriteRTP(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{1},
	})
	require.NoError(t, err)

	<-path.streamCreated

	aw := asyncwriter.New(512, test.NilLogger)

	recv := make(chan struct{})

	path.stream.AddReader(aw,
		path.stream.Desc().Medias[0],
		path.stream.Desc().Medias[0].Formats[0],
		func(u unit.Unit) error {
			select {
			case <-recv:
				return nil
			default:
			}

			require.Equal(t, [][]byte{
				{1},
			}, u.(*unit.H264).AU)
			close(recv)

			return nil
		})

	err = tracks[0].WriteRTP(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 124,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{1},
	})
	require.NoError(t, err)

	aw.Start()
	<-recv
	aw.Stop()
}

func TestServerRead(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

	stream, err := stream.New(
		1460,
		desc,
		true,
		test.NilLogger,
	)
	require.NoError(t, err)

	path := &dummyPath{stream: stream}

	pathManager := &dummyPathManager{path: path}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.StringDuration(10 * time.Second),
		WriteQueueSize:        512,
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers:            []conf.WebRTCICEServer{},
		ExternalCmdPool:       nil,
		PathManager:           pathManager,
		Parent:                test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://myuser:mypass@localhost:8886/teststream/whep?param=value")
	require.NoError(t, err)

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	wc := &webrtc.WHIPClient{
		HTTPClient: hc,
		URL:        u,
		Log:        test.NilLogger,
	}

	writerDone := make(chan struct{})
	defer func() { <-writerDone }()

	writerTerminate := make(chan struct{})
	defer close(writerTerminate)

	go func() {
		defer close(writerDone)
		for {
			select {
			case <-time.After(100 * time.Millisecond):
			case <-writerTerminate:
				return
			}
			stream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
				Base: unit.Base{
					NTP: time.Time{},
				},
				AU: [][]byte{
					{5, 1},
				},
			})
		}
	}()

	tracks, err := wc.Read(context.Background())
	require.NoError(t, err)
	defer checkClose(t, wc.Close)

	pkt, err := tracks[0].ReadRTP()
	require.NoError(t, err)
	require.Equal(t, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    100,
			SequenceNumber: pkt.SequenceNumber,
			Timestamp:      pkt.Timestamp,
			SSRC:           pkt.SSRC,
			CSRC:           []uint32{},
		},
		Payload: []byte{
			0x18, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28, 0xd9,
			0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00,
			0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0,
			0x3c, 0x60, 0xc9, 0x20, 0x00, 0x04, 0x08, 0x06,
			0x07, 0x08, 0x00, 0x02, 0x05, 0x01,
		},
	}, pkt)
}

func TestServerPostNotFound(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	pc, err := pwebrtc.NewPeerConnection(pwebrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close() //nolint:errcheck

	_, err = pc.AddTransceiverFromKind(pwebrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost,
		"http://myuser:mypass@localhost:8886/nonexisting/whep", bytes.NewReader([]byte(offer.SDP)))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/sdp")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestServerPatchNotFound(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	pc, err := pwebrtc.NewPeerConnection(pwebrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close() //nolint:errcheck

	_, err = pc.AddTransceiverFromKind(pwebrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	frag, err := webrtc.ICEFragmentMarshal(offer.SDP, []*pwebrtc.ICECandidateInit{{
		Candidate:     "mycandidate",
		SDPMLineIndex: uint16Ptr(0),
	}})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch,
		"http://localhost:8886/nonexisting/whep/"+uuid.UUID{}.String(), bytes.NewReader(frag))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestServerDeleteNotFound(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodDelete,
		"http://localhost:8886/nonexisting/whep/"+uuid.UUID{}.String(), nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestICEServerNoClientOnly(t *testing.T) {
	s := &Server{
		ICEServers: []conf.WebRTCICEServer{
			{
				URL:      "turn:turn.example.com:1234",
				Username: "user",
				Password: "passwrd",
			},
		},
	}
	clientICEServers, err := s.generateICEServers(true)
	require.NoError(t, err)
	require.Equal(t, len(s.ICEServers), len(clientICEServers))
	serverICEServers, err := s.generateICEServers(false)
	require.NoError(t, err)
	require.Equal(t, len(s.ICEServers), len(serverICEServers))
}

func TestICEServerClientOnly(t *testing.T) {
	s := &Server{
		ICEServers: []conf.WebRTCICEServer{
			{
				URL:        "turn:turn.example.com:1234",
				Username:   "user",
				Password:   "passwrd",
				ClientOnly: true,
			},
		},
	}
	clientICEServers, err := s.generateICEServers(true)
	require.NoError(t, err)
	require.Equal(t, len(s.ICEServers), len(clientICEServers))
	serverICEServers, err := s.generateICEServers(false)
	require.NoError(t, err)
	require.Equal(t, 0, len(serverICEServers))
}
