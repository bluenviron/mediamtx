package srt

import (
	"bufio"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	srt "github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"
)

type dummyPath struct{}

func (p *dummyPath) Name() string {
	return "teststream"
}

func (p *dummyPath) SafeConf() *conf.Path {
	return &conf.Path{}
}

func (p *dummyPath) ExternalCmdEnv() externalcmd.Environment {
	return externalcmd.Environment{}
}

func (p *dummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {
}

func (p *dummyPath) RemoveReader(_ defs.PathRemoveReaderReq) {
}

func TestServerPublish(t *testing.T) {
	externalCmdPool := &externalcmd.Pool{}
	externalCmdPool.Initialize()
	defer externalCmdPool.Close()

	var strm *stream.Stream
	streamCreated := make(chan struct{})

	pathManager := &test.PathManager{
		FindPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
			require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
			return &conf.Path{}, nil
		},
		AddPublisherImpl: func(req defs.PathAddPublisherReq) (defs.Path, *stream.Stream, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.True(t, req.AccessRequest.SkipAuth)

			strm = &stream.Stream{
				WriteQueueSize:     512,
				RTPMaxPayloadSize:  1450,
				Desc:               req.Desc,
				GenerateRTPPackets: true,
				Parent:             test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)

			close(streamCreated)

			return &dummyPath{}, strm, nil
		},
	}

	s := &Server{
		Address:             "127.0.0.1:8890",
		RTSPAddress:         "",
		ReadTimeout:         conf.Duration(10 * time.Second),
		WriteTimeout:        conf.Duration(10 * time.Second),
		UDPMaxPayloadSize:   1472,
		RunOnConnect:        "",
		RunOnConnectRestart: false,
		RunOnDisconnect:     "string",
		ExternalCmdPool:     externalCmdPool,
		PathManager:         pathManager,
		Parent:              test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u := "srt://127.0.0.1:8890?streamid=publish:teststream:myuser:mypass:param=value"

	srtConf := srt.DefaultConfig()
	address, err := srtConf.UnmarshalURL(u)
	require.NoError(t, err)

	err = srtConf.Validate()
	require.NoError(t, err)

	publisher, err := srt.Dial("srt", address, srtConf)
	require.NoError(t, err)
	defer publisher.Close()

	track := &mpegts.Track{
		Codec: &mpegts.CodecH264{},
	}

	bw := bufio.NewWriter(publisher)
	w := &mpegts.Writer{W: bw, Tracks: []*mpegts.Track{track}}
	err = w.Initialize()
	require.NoError(t, err)

	err = w.WriteH264(track, 0, 0, [][]byte{
		test.FormatH264.SPS,
		test.FormatH264.PPS,
		{0x05, 1}, // IDR
	})
	require.NoError(t, err)

	err = bw.Flush()
	require.NoError(t, err)

	<-streamCreated

	reader := test.NilLogger

	recv := make(chan struct{})

	strm.AddReader(
		reader,
		strm.Desc.Medias[0],
		strm.Desc.Medias[0].Formats[0],
		func(u unit.Unit) error {
			require.Equal(t, [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{0x05, 1}, // IDR
			}, u.(*unit.H264).AU)
			close(recv)
			return nil
		})

	strm.StartReader(reader)
	defer strm.RemoveReader(reader)

	err = w.WriteH264(track, 0, 0, [][]byte{
		{5, 2},
	})
	require.NoError(t, err)

	err = bw.Flush()
	require.NoError(t, err)

	<-recv
}

func TestServerRead(t *testing.T) {
	externalCmdPool := &externalcmd.Pool{}
	externalCmdPool.Initialize()
	defer externalCmdPool.Close()

	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

	strm := &stream.Stream{
		WriteQueueSize:     512,
		RTPMaxPayloadSize:  1450,
		Desc:               desc,
		GenerateRTPPackets: true,
		Parent:             test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	pathManager := &test.PathManager{
		AddReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
			require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
			return &dummyPath{}, strm, nil
		},
	}

	s := &Server{
		Address:             "127.0.0.1:8890",
		RTSPAddress:         "",
		ReadTimeout:         conf.Duration(10 * time.Second),
		WriteTimeout:        conf.Duration(10 * time.Second),
		UDPMaxPayloadSize:   1472,
		RunOnConnect:        "",
		RunOnConnectRestart: false,
		RunOnDisconnect:     "string",
		ExternalCmdPool:     externalCmdPool,
		PathManager:         pathManager,
		Parent:              test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u := "srt://127.0.0.1:8890?streamid=read:teststream:myuser:mypass:param=value"

	srtConf := srt.DefaultConfig()
	address, err := srtConf.UnmarshalURL(u)
	require.NoError(t, err)

	err = srtConf.Validate()
	require.NoError(t, err)

	reader, err := srt.Dial("srt", address, srtConf)
	require.NoError(t, err)
	defer reader.Close()

	strm.WaitRunningReader()

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
		Base: unit.Base{
			NTP: time.Time{},
		},
		AU: [][]byte{
			{5, 1}, // IDR
		},
	})

	r := &mpegts.Reader{R: reader}
	err = r.Initialize()
	require.NoError(t, err)

	require.Equal(t, []*mpegts.Track{{
		PID:   256,
		Codec: &mpegts.CodecH264{},
	}}, r.Tracks())

	received := false

	r.OnDataH264(r.Tracks()[0], func(pts int64, dts int64, au [][]byte) error {
		require.Equal(t, int64(0), pts)
		require.Equal(t, int64(0), dts)
		require.Equal(t, [][]byte{
			test.FormatH264.SPS,
			test.FormatH264.PPS,
			{0x05, 1},
		}, au)
		received = true
		return nil
	})

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
		Base: unit.Base{
			NTP: time.Time{},
		},
		AU: [][]byte{
			{5, 2},
		},
	})

	for {
		err = r.Read()
		require.NoError(t, err)
		if received {
			break
		}
	}
}
