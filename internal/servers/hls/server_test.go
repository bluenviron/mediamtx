package hls

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

type dummyPath struct{}

func (pa *dummyPath) Name() string {
	return "mystream"
}

func (pa *dummyPath) SafeConf() *conf.Path {
	return &conf.Path{}
}

func (pa *dummyPath) ExternalCmdEnv() externalcmd.Environment {
	return nil
}

func (pa *dummyPath) StartPublisher(_ defs.PathStartPublisherReq) (*stream.Stream, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (pa *dummyPath) StopPublisher(_ defs.PathStopPublisherReq) {
}

func (pa *dummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {
}

func (pa *dummyPath) RemoveReader(_ defs.PathRemoveReaderReq) {
}

type dummyPathManager struct {
	stream *stream.Stream
}

func (pm *dummyPathManager) FindPathConf(_ defs.PathFindPathConfReq) (*conf.Path, error) {
	return &conf.Path{}, nil
}

func (pm *dummyPathManager) AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
	if req.AccessRequest.Name == "nonexisting" {
		return nil, nil, fmt.Errorf("not found")
	}
	return &dummyPath{}, pm.stream, nil
}

func TestServerNotFound(t *testing.T) {
	for _, ca := range []string{
		"always remux off",
		"always remux on",
	} {
		t.Run(ca, func(t *testing.T) {
			s := &Server{
				Address:                   "127.0.0.1:8888",
				Encryption:                false,
				ServerKey:                 "",
				ServerCert:                "",
				ExternalAuthenticationURL: "",
				AlwaysRemux:               ca == "always remux on",
				Variant:                   conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
				SegmentCount:              7,
				SegmentDuration:           conf.StringDuration(1 * time.Second),
				PartDuration:              conf.StringDuration(200 * time.Millisecond),
				SegmentMaxSize:            50 * 1024 * 1024,
				AllowOrigin:               "",
				TrustedProxies:            conf.IPsOrCIDRs{},
				Directory:                 "",
				ReadTimeout:               conf.StringDuration(10 * time.Second),
				WriteQueueSize:            512,
				PathManager:               &dummyPathManager{},
				Parent:                    &test.NilLogger{},
			}
			err := s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			hc := &http.Client{Transport: &http.Transport{}}

			func() {
				req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/nonexisting/", nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()
				require.Equal(t, http.StatusOK, res.StatusCode)
			}()

			func() {
				req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/nonexisting/index.m3u8", nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()
				require.Equal(t, http.StatusNotFound, res.StatusCode)
			}()
		})
	}
}

func TestServerRead(t *testing.T) {
	t.Run("always remux off", func(t *testing.T) {
		desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

		stream, err := stream.New(
			1460,
			desc,
			true,
			test.NilLogger{},
		)
		require.NoError(t, err)

		pathManager := &dummyPathManager{stream: stream}

		s := &Server{
			Address:                   "127.0.0.1:8888",
			Encryption:                false,
			ServerKey:                 "",
			ServerCert:                "",
			ExternalAuthenticationURL: "",
			AlwaysRemux:               false,
			Variant:                   conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
			SegmentCount:              7,
			SegmentDuration:           conf.StringDuration(1 * time.Second),
			PartDuration:              conf.StringDuration(200 * time.Millisecond),
			SegmentMaxSize:            50 * 1024 * 1024,
			AllowOrigin:               "",
			TrustedProxies:            conf.IPsOrCIDRs{},
			Directory:                 "",
			ReadTimeout:               conf.StringDuration(10 * time.Second),
			WriteQueueSize:            512,
			PathManager:               pathManager,
			Parent:                    &test.NilLogger{},
		}
		err = s.Initialize()
		require.NoError(t, err)
		defer s.Close()

		c := &gohlslib.Client{
			URI: "http://127.0.0.1:8888/mystream/index.m3u8",
		}

		recv := make(chan struct{})

		c.OnTracks = func(tracks []*gohlslib.Track) error {
			require.Equal(t, []*gohlslib.Track{{
				Codec: &codecs.H264{},
			}}, tracks)

			c.OnDataH26x(tracks[0], func(pts, dts time.Duration, au [][]byte) {
				require.Equal(t, time.Duration(0), pts)
				require.Equal(t, time.Duration(0), dts)
				require.Equal(t, [][]byte{
					{byte(h264.NALUTypeAccessUnitDelimiter), 0xf0},
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5, 1},
				}, au)
				close(recv)
			})

			return nil
		}

		err = c.Start()
		require.NoError(t, err)
		defer func() { <-c.Wait() }()
		defer c.Close()

		go func() {
			time.Sleep(100 * time.Millisecond)
			for i := 0; i < 4; i++ {
				stream.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
						PTS: time.Duration(i) * time.Second,
					},
					AU: [][]byte{
						{5, 1}, // IDR
					},
				})
			}
		}()

		<-recv
	})

	t.Run("always remux on", func(t *testing.T) {
		desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

		stream, err := stream.New(
			1460,
			desc,
			true,
			test.NilLogger{},
		)
		require.NoError(t, err)

		pathManager := &dummyPathManager{stream: stream}

		s := &Server{
			Address:                   "127.0.0.1:8888",
			Encryption:                false,
			ServerKey:                 "",
			ServerCert:                "",
			ExternalAuthenticationURL: "",
			AlwaysRemux:               true,
			Variant:                   conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
			SegmentCount:              7,
			SegmentDuration:           conf.StringDuration(1 * time.Second),
			PartDuration:              conf.StringDuration(200 * time.Millisecond),
			SegmentMaxSize:            50 * 1024 * 1024,
			AllowOrigin:               "",
			TrustedProxies:            conf.IPsOrCIDRs{},
			Directory:                 "",
			ReadTimeout:               conf.StringDuration(10 * time.Second),
			WriteQueueSize:            512,
			PathManager:               pathManager,
			Parent:                    &test.NilLogger{},
		}
		err = s.Initialize()
		require.NoError(t, err)
		defer s.Close()

		s.PathReady(&dummyPath{})

		time.Sleep(100 * time.Millisecond)

		for i := 0; i < 4; i++ {
			stream.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
				Base: unit.Base{
					NTP: time.Time{},
					PTS: time.Duration(i) * time.Second,
				},
				AU: [][]byte{
					{5, 1}, // IDR
				},
			})
		}

		c := &gohlslib.Client{
			URI: "http://127.0.0.1:8888/mystream/index.m3u8",
		}

		recv := make(chan struct{})

		c.OnTracks = func(tracks []*gohlslib.Track) error {
			require.Equal(t, []*gohlslib.Track{{
				Codec: &codecs.H264{},
			}}, tracks)

			c.OnDataH26x(tracks[0], func(pts, dts time.Duration, au [][]byte) {
				require.Equal(t, time.Duration(0), pts)
				require.Equal(t, time.Duration(0), dts)
				require.Equal(t, [][]byte{
					{0x09, 0xf0},
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5, 1},
				}, au)
				close(recv)
			})

			return nil
		}

		err = c.Start()
		require.NoError(t, err)
		defer func() { <-c.Wait() }()
		defer c.Close()

		<-recv
	})
}
