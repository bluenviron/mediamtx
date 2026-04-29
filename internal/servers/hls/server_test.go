package hls

import (
	"encoding/json"
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
	"github.com/bluenviron/gortsplib/v5/pkg/format"
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
	findPathConfImpl func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error)
	addReaderImpl    func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

func (pm *dummyPathManager) SetHLSServer(*Server) []defs.Path {
	if pm.setHLSServerImpl != nil {
		return pm.setHLSServerImpl()
	}
	return nil
}

func (pm *dummyPathManager) FindPathConf(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
	return pm.findPathConfImpl(req)
}

func (pm *dummyPathManager) AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
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

func TestServerIndexNotConfigured(t *testing.T) {
	pm := &dummyPathManager{
		findPathConfImpl: func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
			require.Equal(t, "nonconfigured", req.AccessRequest.Name)
			return nil, fmt.Errorf("path is not configured")
		},
	}

	s := &Server{
		Address:      "127.0.0.1:8888",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		PathManager:  pm,
		Parent:       test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888/nonconfigured/", nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusInternalServerError, res.StatusCode)
	require.Contains(t, res.Header.Get("Content-Type"), "application/json")

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	var payload defs.APIError
	err = json.Unmarshal(byts, &payload)
	require.NoError(t, err)
	require.Equal(t, defs.APIError{
		Status: defs.APIErrorStatusError,
		Error:  "path is not configured",
	}, payload)
}

func TestServerIndexRedirect(t *testing.T) {
	s := &Server{
		Address:      "127.0.0.1:8888",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		PathManager:  &dummyPathManager{},
		Parent:       test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	res, err := client.Get("http://127.0.0.1:8888/stream")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusFound, res.StatusCode)
	require.Equal(t, "/stream/", res.Header.Get("Location"))
}

func TestServerIndexRedirectNoXSS(t *testing.T) {
	for _, ca := range []struct {
		name     string
		path     string
		expected string
	}{
		{"double slash", "//double/slash", "/double/slash/"},
		{"protocol-relative open redirect", "//evil.com", "/evil.com/"},
		{"backslash bypass", "/%5Cevil.com", "/evil.com/"},
	} {
		t.Run(ca.name, func(t *testing.T) {
			pm := &dummyPathManager{
				findPathConfImpl: func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}, User: req.AccessRequest.Credentials.User}, nil
				},
			}

			s := &Server{
				Address:      "127.0.0.1:8888",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				PathManager:  pm,
				Parent:       test.NilLogger,
			}
			err := s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{
				Transport: tr,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8888"+ca.path, nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusFound, res.StatusCode)
			require.Equal(t, ca.expected, res.Header.Get("Location"))
		})
	}
}

func TestServerNotFound(t *testing.T) {
	for _, ca := range []string{
		"always remux off",
		"always remux on",
	} {
		t.Run(ca, func(t *testing.T) {
			pm := &dummyPathManager{
				findPathConfImpl: func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
					require.Equal(t, "nonexisting", req.AccessRequest.Name)
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}, User: req.AccessRequest.Credentials.User}, nil
				},
				addReaderImpl: func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					require.Equal(t, "nonexisting", req.AccessRequest.Name)
					return nil, &defs.PathNoStreamAvailableError{}
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
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				ReplaceNTP:        false,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)

			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			pm := &dummyPathManager{
				findPathConfImpl: func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
					require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}, User: req.AccessRequest.Credentials.User}, nil
				},
				addReaderImpl: func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)

					switch req.Author.(type) {
					case (*muxer):
						if ca == "always remux off" {
							require.Equal(t, "param=value", req.AccessRequest.Query)
						} else {
							require.Equal(t, "", req.AccessRequest.Query)
						}

					case *session:
						require.Equal(t, "param=value", req.AccessRequest.Query)

					default:
						t.Errorf("should not happen")
					}

					return &defs.PathAddReaderRes{Path: &dummyPath{}, Stream: strm}, nil
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
									Type:          2,
									ChannelCount:  2,
									ChannelConfig: 2,
									SampleRate:    44100,
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
					subStream.WriteUnit(test.MediaH264, test.FormatH264, &unit.Unit{
						NTP: time.Time{},
						PTS: int64(i) * 90000,
						Payload: unit.PayloadH264{
							{5, 1}, // IDR
						},
					})
					subStream.WriteUnit(test.MediaMPEG4Audio, test.FormatMPEG4Audio, &unit.Unit{
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
					subStream.WriteUnit(test.MediaH264, test.FormatH264, &unit.Unit{
						NTP: time.Time{},
						PTS: int64(i) * 90000,
						Payload: unit.PayloadH264{
							{5, 1}, // IDR
						},
					})
					subStream.WriteUnit(test.MediaMPEG4Audio, test.FormatMPEG4Audio, &unit.Unit{
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
									Type:          2,
									ChannelCount:  2,
									ChannelConfig: 2,
									SampleRate:    44100,
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
	dir := t.TempDir()

	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

	strm := &stream.Stream{
		Desc:              desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = subStream.Initialize()
	require.NoError(t, err)

	pm := &dummyPathManager{
		addReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
			return &defs.PathAddReaderRes{Path: &dummyPath{}, Stream: strm}, nil
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
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = subStream.Initialize()
	require.NoError(t, err)

	done := make(chan struct{})

	pm := &dummyPathManager{
		setHLSServerImpl: func() []defs.Path {
			return []defs.Path{&dummyPath{}}
		},
		addReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
			close(done)
			return &defs.PathAddReaderRes{Path: &dummyPath{}, Stream: strm}, nil
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
			addReaderImpl: func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
				if req.AccessRequest.Credentials.User == "" && req.AccessRequest.Credentials.Pass == "" {
					return nil, &auth.Error{AskCredentials: true, Wrapped: fmt.Errorf("auth error")}
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

func TestAuthQueryPreservedAcrossRedirect(t *testing.T) {
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
			findPathConfImpl: func(_ defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
				return nil, &auth.Error{AskCredentials: true, Wrapped: fmt.Errorf("auth error")}
			},
		},
		Parent: test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	res, err := client.Get("http://127.0.0.1:8888/stream/index.m3u8?jwt=mytoken")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusFound, res.StatusCode)
	require.Equal(t, "/stream/index.m3u8?cookieCheck=1&jwt=mytoken", res.Header.Get("Location"))
}

func TestServerNoSupportedCodecs(t *testing.T) {
	for _, ca := range []string{
		"always remux off",
		"always remux on",
	} {
		t.Run(ca, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.VP8{}},
			}}}

			strm := &stream.Stream{
				Desc:              desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)

			pm := &dummyPathManager{
				findPathConfImpl: func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}, User: req.AccessRequest.Credentials.User}, nil
				},
				addReaderImpl: func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					return &defs.PathAddReaderRes{Path: &dummyPath{}, Stream: strm}, nil
				},
			}

			s := &Server{
				Address:         "127.0.0.1:8888",
				AlwaysRemux:     (ca == "always remux on"),
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

			client := &http.Client{
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}

			res, err := client.Get("http://127.0.0.1:8888/teststream/index.m3u8")
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusFound, res.StatusCode)
			require.Equal(t, "/teststream/index.m3u8?cookieCheck=1", res.Header.Get("Location"))

			res, err = client.Get("http://127.0.0.1:8888/teststream/index.m3u8?cookieCheck=1")
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusInternalServerError, res.StatusCode)
			require.Contains(t, res.Header.Get("Content-Type"), "application/json")

			byts, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			var payload defs.APIError
			err = json.Unmarshal(byts, &payload)
			require.NoError(t, err)

			if ca == "always remux off" {
				require.Equal(t, defs.APIError{
					Status: defs.APIErrorStatusError,
					Error:  "terminated",
				}, payload)
			} else {
				require.Equal(t, defs.APIError{
					Status: defs.APIErrorStatusError,
					Error:  "muxer is waiting to be created",
				}, payload)
			}
		})
	}
}
