package rtsp

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

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
		512,
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

func (pm *dummyPathManager) Describe(_ defs.PathDescribeReq) defs.PathDescribeRes {
	return defs.PathDescribeRes{
		Path:     pm.path,
		Stream:   pm.path.stream,
		Redirect: "",
		Err:      nil,
	}
}

func (pm *dummyPathManager) AddPublisher(_ defs.PathAddPublisherReq) (defs.Path, error) {
	return pm.path, nil
}

func (pm *dummyPathManager) AddReader(_ defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
	return pm.path, pm.path.stream, nil
}

func TestServerPublish(t *testing.T) {
	path := &dummyPath{
		streamCreated: make(chan struct{}),
	}

	pathManager := &dummyPathManager{path: path}

	s := &Server{
		Address:             "127.0.0.1:8557",
		AuthMethods:         []auth.ValidateMethod{auth.ValidateMethodBasic},
		ReadTimeout:         conf.StringDuration(10 * time.Second),
		WriteTimeout:        conf.StringDuration(10 * time.Second),
		WriteQueueSize:      512,
		UseUDP:              false,
		UseMulticast:        false,
		RTPAddress:          "",
		RTCPAddress:         "",
		MulticastIPRange:    "",
		MulticastRTPPort:    0,
		MulticastRTCPPort:   0,
		IsTLS:               false,
		ServerCert:          "",
		ServerKey:           "",
		RTSPAddress:         "",
		Protocols:           map[conf.Protocol]struct{}{conf.Protocol(gortsplib.TransportTCP): {}},
		RunOnConnect:        "",
		RunOnConnectRestart: false,
		RunOnDisconnect:     "",
		ExternalCmdPool:     nil,
		PathManager:         pathManager,
		Parent:              test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	source := gortsplib.Client{}

	media0 := test.UniqueMediaH264()

	err = source.StartRecording(
		"rtsp://myuser:mypass@127.0.0.1:8557/teststream?param=value",
		&description.Session{Medias: []*description.Media{media0}})
	require.NoError(t, err)
	defer source.Close()

	<-path.streamCreated

	reader := test.NilLogger

	recv := make(chan struct{})

	path.stream.AddReader(
		reader,
		path.stream.Desc().Medias[0],
		path.stream.Desc().Medias[0].Formats[0],
		func(u unit.Unit) error {
			require.Equal(t, [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5, 2, 3, 4},
			}, u.(*unit.H264).AU)
			close(recv)
			return nil
		})

	path.stream.StartReader(reader)
	defer path.stream.RemoveReader(reader)

	err = source.WritePacketRTP(media0, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{5, 2, 3, 4},
	})
	require.NoError(t, err)

	<-recv
}

func TestServerRead(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

	stream, err := stream.New(
		512,
		1460,
		desc,
		true,
		test.NilLogger,
	)
	require.NoError(t, err)

	path := &dummyPath{stream: stream}

	pathManager := &dummyPathManager{path: path}

	s := &Server{
		Address:             "127.0.0.1:8557",
		AuthMethods:         []auth.ValidateMethod{auth.ValidateMethodBasic},
		ReadTimeout:         conf.StringDuration(10 * time.Second),
		WriteTimeout:        conf.StringDuration(10 * time.Second),
		WriteQueueSize:      512,
		UseUDP:              false,
		UseMulticast:        false,
		RTPAddress:          "",
		RTCPAddress:         "",
		MulticastIPRange:    "",
		MulticastRTPPort:    0,
		MulticastRTCPPort:   0,
		IsTLS:               false,
		ServerCert:          "",
		ServerKey:           "",
		RTSPAddress:         "",
		Protocols:           map[conf.Protocol]struct{}{conf.Protocol(gortsplib.TransportTCP): {}},
		RunOnConnect:        "",
		RunOnConnectRestart: false,
		RunOnDisconnect:     "",
		ExternalCmdPool:     nil,
		PathManager:         pathManager,
		Parent:              test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	reader := gortsplib.Client{}

	u, err := base.ParseURL("rtsp://myuser:mypass@127.0.0.1:8557/teststream?param=value")
	require.NoError(t, err)

	err = reader.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer reader.Close()

	desc2, _, err := reader.Describe(u)
	require.NoError(t, err)

	err = reader.SetupAll(desc2.BaseURL, desc2.Medias)
	require.NoError(t, err)

	recv := make(chan struct{})

	reader.OnPacketRTPAny(func(_ *description.Media, _ format.Format, p *rtp.Packet) {
		require.Equal(t, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: p.SequenceNumber,
				Timestamp:      p.Timestamp,
				SSRC:           p.SSRC,
				CSRC:           []uint32{},
			},
			Payload: []byte{
				0x18, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28, 0xd9,
				0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00,
				0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0,
				0x3c, 0x60, 0xc9, 0x20, 0x00, 0x04, 0x08, 0x06,
				0x07, 0x08, 0x00, 0x04, 0x05, 0x02, 0x03, 0x04,
			},
		}, p)
		close(recv)
	})

	_, err = reader.Play(nil)
	require.NoError(t, err)

	stream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
		Base: unit.Base{
			NTP: time.Time{},
		},
		AU: [][]byte{
			{5, 2, 3, 4}, // IDR
		},
	})

	<-recv
}
