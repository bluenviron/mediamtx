package hls

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

type dummyPathManager struct {
	setHLSServerImpl func() []defs.Path
	findPathConfImpl func(req defs.PathFindPathConfReq) (*conf.Path, error)
	addReaderImpl    func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error)
}

func (pm *dummyPathManager) SetHLSServer(*Server) []defs.Path {
	if pm.setHLSServerImpl != nil {
		return pm.setHLSServerImpl()
	}
	return nil
}

func (pm *dummyPathManager) FindPathConf(req defs.PathFindPathConfReq) (*conf.Path, error) {
	return pm.findPathConfImpl(req)
}

func (pm *dummyPathManager) AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
	return pm.addReaderImpl(req)
}

type dummyPath struct{}

func (pa *dummyPath) Name() string {
	return "teststream"
}

func (pa *dummyPath) SafeConf() *conf.Path {
	return &conf.Path{}
}

func (pa *dummyPath) ExternalCmdEnv() externalcmd.Environment {
	return nil
}

func (pa *dummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {
}

func (pa *dummyPath) RemoveReader(_ defs.PathRemoveReaderReq) {
}

func TestServerPreflightRequest(t *testing.T) {
	s := &Server{
		Address:      "127.0.0.1:8888",
		AllowOrigins: []string{"*"},
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		PathManager:  &dummyPathManager{},
		Parent:       test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions, "http://localhost:8888", nil)
	require.NoError(t, err)

	req.Header.Add("Access-Control-Request-Method", "GET")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, "*", res.Header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", res.Header.Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "OPTIONS, GET", res.Header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Authorization, Range", res.Header.Get("Access-Control-Allow-Headers"))
	require.Equal(t, byts, []byte{})
}

func TestServerNotFound(t *testing.T) {
	for _, ca := range []string{
		"always remux off",
		"always remux on",
	} {
		t.Run(ca, func(t *testing.T) {
			pm := &dummyPathManager{
				findPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
					require.Equal(t, "nonexisting", req.AccessRequest.Name)
					return &conf.Path{}, nil
				},
				addReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
					require.Equal(t, "nonexisting", req.AccessRequest.Name)
					return nil, nil, fmt.Errorf("not found")
				},
			}

			s := &Server{
				Address:         "127.0.0.1:8888",
				Encryption:      false,
				ServerKey:       "",
				ServerCert:      "",
				AlwaysRemux:     ca == "always remux on",
				Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
				SegmentCount:    7,
				SegmentDuration: conf.Duration(1 * time.Second),
				PartDuration:    conf.Duration(200 * time.Millisecond),
				SegmentMaxSize:  50 * 1024 * 1024,
				TrustedProxies:  conf.IPNetworks{},
				Directory:       "",
				ReadTimeout:     conf.Duration(10 * time.Second),
				WriteTimeout:    conf.Duration(10 * time.Second),
				PathManager:     pm,
				Parent:          test.NilLogger,
			}
			err := s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			func() {
				var req *http.Request
				req, err = http.NewRequest(http.MethodGet, "http://myuser:mypass@127.0.0.1:8888/nonexisting/", nil)
				require.NoError(t, err)

				var res *http.Response
				res, err = hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()
				require.Equal(t, http.StatusOK, res.StatusCode)
			}()

			func() {
				var req *http.Request
				req, err = http.NewRequest(http.MethodGet, "http://myuser:mypass@127.0.0.1:8888/nonexisting/index.m3u8", nil)
				require.NoError(t, err)

				var res *http.Response
				res, err = hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()
				require.Equal(t, http.StatusNotFound, res.StatusCode)
			}()
		})
	}
}

func TestServerRead(t *testing.T) {
	for _, ca := range []string{
		"always remux off",
		"always remux on",
	} {
		t.Run(ca, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{
				test.MediaH264,
				test.MediaMPEG4Audio,
			}}

			strm := &stream.Stream{
				Desc:              desc,
				UseRTPPackets:     false,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				ReplaceNTP:        false,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)

			pm := &dummyPathManager{
				findPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
					require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
					return &conf.Path{}, nil
				},
				addReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					if ca == "always remux off" {
						require.Equal(t, "param=value", req.AccessRequest.Query)
					} else {
						require.Equal(t, "", req.AccessRequest.Query)
					}
					return &dummyPath{}, strm, nil
				},
			}

			switch ca {
			case "always remux off":
				s := &Server{
					Address:         "127.0.0.1:8888",
					AlwaysRemux:     false,
					Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
					SegmentCount:    7,
					SegmentDuration: conf.Duration(1 * time.Second),
					PartDuration:    conf.Duration(200 * time.Millisecond),
					SegmentMaxSize:  50 * 1024 * 1024,
					TrustedProxies:  conf.IPNetworks{},
					ReadTimeout:     conf.Duration(10 * time.Second),
					WriteTimeout:    conf.Duration(10 * time.Second),
					PathManager:     pm,
					Parent:          test.NilLogger,
				}
				err = s.Initialize()
				require.NoError(t, err)
				defer s.Close()

				c := &gohlslib.Client{
					URI:           "http://myuser:mypass@127.0.0.1:8888/teststream/index.m3u8?param=value",
					StartDistance: 1,
				}

				recv1 := make(chan struct{})
				recv2 := make(chan struct{})

				c.OnTracks = func(tracks []*gohlslib.Track) error { //nolint:dupl
					require.Equal(t, []*gohlslib.Track{
						{
							Codec:     &codecs.H264{},
							ClockRate: 90000,
						},
						{
							Codec: &codecs.MPEG4Audio{
								Config: mpeg4audio.AudioSpecificConfig{
									Type:         2,
									ChannelCount: 2,
									SampleRate:   44100,
								},
							},
							ClockRate: 90000,
						},
					}, tracks)

					c.OnDataH26x(tracks[0], func(pts, dts int64, au [][]byte) {
						require.Equal(t, int64(0), pts)
						require.Equal(t, int64(0), dts)
						require.Equal(t, [][]byte{
							test.FormatH264.SPS,
							test.FormatH264.PPS,
							{5, 1},
						}, au)
						close(recv1)
					})

					c.OnDataMPEG4Audio(tracks[1], func(pts int64, aus [][]byte) {
						require.Equal(t, int64(0), pts)
						require.Equal(t, [][]byte{{1, 2}}, aus)
						close(recv2)
					})

					return nil
				}

				err = c.Start()
				require.NoError(t, err)
				defer c.Close()

				strm.WaitForReaders()

				for i := range 2 {
					strm.WriteUnit(test.MediaH264, test.FormatH264, &unit.Unit{
						NTP: time.Time{},
						PTS: int64(i) * 90000,
						Payload: unit.PayloadH264{
							{5, 1}, // IDR
						},
					})
					strm.WriteUnit(test.MediaMPEG4Audio, test.FormatMPEG4Audio, &unit.Unit{
						NTP:     time.Time{},
						PTS:     int64(i) * 44100,
						Payload: unit.PayloadMPEG4Audio{{1, 2}},
					})
				}

				<-recv1
				<-recv2

			case "always remux on":
				s := &Server{
					Address:         "127.0.0.1:8888",
					AlwaysRemux:     true,
					Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
					SegmentCount:    7,
					SegmentDuration: conf.Duration(1 * time.Second),
					PartDuration:    conf.Duration(200 * time.Millisecond),
					SegmentMaxSize:  50 * 1024 * 1024,
					TrustedProxies:  conf.IPNetworks{},
					ReadTimeout:     conf.Duration(10 * time.Second),
					WriteTimeout:    conf.Duration(10 * time.Second),
					PathManager:     pm,
					Parent:          test.NilLogger,
				}
				err = s.Initialize()
				require.NoError(t, err)
				defer s.Close()

				s.PathReady(&dummyPath{})

				strm.WaitForReaders()

				for i := range 2 {
					strm.WriteUnit(test.MediaH264, test.FormatH264, &unit.Unit{
						NTP: time.Time{},
						PTS: int64(i) * 90000,
						Payload: unit.PayloadH264{
							{5, 1}, // IDR
						},
					})
					strm.WriteUnit(test.MediaMPEG4Audio, test.FormatMPEG4Audio, &unit.Unit{
						NTP:     time.Time{},
						PTS:     int64(i) * 44100,
						Payload: unit.PayloadMPEG4Audio{{1, 2}},
					})
				}

				c := &gohlslib.Client{
					URI:           "http://myuser:mypass@127.0.0.1:8888/teststream/index.m3u8?param=value",
					StartDistance: 1,
				}

				recv1 := make(chan struct{})
				recv2 := make(chan struct{})

				c.OnTracks = func(tracks []*gohlslib.Track) error { //nolint:dupl
					require.Equal(t, []*gohlslib.Track{
						{
							Codec:     &codecs.H264{},
							ClockRate: 90000,
						},
						{
							Codec: &codecs.MPEG4Audio{
								Config: mpeg4audio.AudioSpecificConfig{
									Type:         2,
									ChannelCount: 2,
									SampleRate:   44100,
								},
							},
							ClockRate: 90000,
						},
					}, tracks)

					c.OnDataH26x(tracks[0], func(pts, dts int64, au [][]byte) {
						require.Equal(t, int64(0), pts)
						require.Equal(t, int64(0), dts)
						require.Equal(t, [][]byte{
							test.FormatH264.SPS,
							test.FormatH264.PPS,
							{5, 1},
						}, au)
						close(recv1)
					})

					c.OnDataMPEG4Audio(tracks[1], func(pts int64, aus [][]byte) {
						require.Equal(t, int64(0), pts)
						require.Equal(t, [][]byte{{1, 2}}, aus)
						close(recv2)
					})

					return nil
				}

				err = c.Start()
				require.NoError(t, err)
				defer c.Close()

				<-recv1
				<-recv2
			}
		})
	}
}

func TestServerDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

	strm := &stream.Stream{
		Desc:              desc,
		UseRTPPackets:     false,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err = strm.Initialize()
	require.NoError(t, err)

	pm := &dummyPathManager{
		addReaderImpl: func(_ defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
			return &dummyPath{}, strm, nil
		},
	}

	s := &Server{
		Address:         "127.0.0.1:8888",
		Encryption:      false,
		ServerKey:       "",
		ServerCert:      "",
		AlwaysRemux:     true,
		Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
		SegmentCount:    7,
		SegmentDuration: conf.Duration(1 * time.Second),
		PartDuration:    conf.Duration(200 * time.Millisecond),
		SegmentMaxSize:  50 * 1024 * 1024,
		TrustedProxies:  conf.IPNetworks{},
		Directory:       filepath.Join(dir, "mydir"),
		ReadTimeout:     conf.Duration(10 * time.Second),
		WriteTimeout:    conf.Duration(10 * time.Second),
		PathManager:     pm,
		Parent:          test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	s.PathReady(&dummyPath{})

	time.Sleep(100 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "mydir", "teststream"))
	require.NoError(t, err)
}

func TestServerDynamicAlwaysRemux(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

	strm := &stream.Stream{
		Desc:              desc,
		UseRTPPackets:     false,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	done := make(chan struct{})

	pm := &dummyPathManager{
		setHLSServerImpl: func() []defs.Path {
			return []defs.Path{&dummyPath{}}
		},
		addReaderImpl: func(_ defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
			close(done)
			return &dummyPath{}, strm, nil
		},
	}

	s := &Server{
		Address:         "127.0.0.1:8888",
		Encryption:      false,
		ServerKey:       "",
		ServerCert:      "",
		AlwaysRemux:     true,
		Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
		SegmentCount:    7,
		SegmentDuration: conf.Duration(1 * time.Second),
		PartDuration:    conf.Duration(200 * time.Millisecond),
		SegmentMaxSize:  50 * 1024 * 1024,
		ReadTimeout:     conf.Duration(10 * time.Second),
		WriteTimeout:    conf.Duration(10 * time.Second),
		PathManager:     pm,
		Parent:          test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	<-done
}

func TestAuthError(t *testing.T) {
	n := 0

	s := &Server{
		Address:         "127.0.0.1:8888",
		Encryption:      false,
		ServerKey:       "",
		ServerCert:      "",
		AlwaysRemux:     true,
		Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
		SegmentCount:    7,
		SegmentDuration: conf.Duration(1 * time.Second),
		PartDuration:    conf.Duration(200 * time.Millisecond),
		SegmentMaxSize:  50 * 1024 * 1024,
		ReadTimeout:     conf.Duration(10 * time.Second),
		WriteTimeout:    conf.Duration(10 * time.Second),
		PathManager: &dummyPathManager{
			findPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
				if req.AccessRequest.Credentials.User == "" && req.AccessRequest.Credentials.Pass == "" {
					return nil, &auth.Error{AskCredentials: true}
				}

				return nil, &auth.Error{Wrapped: fmt.Errorf("auth error")}
			},
		},
		Parent: test.Logger(func(l logger.Level, s string, i ...any) {
			if l == logger.Info {
				if n == 1 {
					require.Regexp(t, "failed to authenticate: auth error$", fmt.Sprintf(s, i...))
				}
				n++
			}
		}),
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/stream/index.m3u8", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	require.Equal(t, `Basic realm="mediamtx"`, res.Header.Get("WWW-Authenticate"))

	req, err = http.NewRequest(http.MethodGet, "http://myuser:mypass@127.0.0.1:8888/stream/index.m3u8", nil)
	require.NoError(t, err)

	start := time.Now()

	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Greater(t, time.Since(start), 2*time.Second)

	require.Equal(t, http.StatusUnauthorized, res.StatusCode)

	require.Equal(t, 2, n)
}
