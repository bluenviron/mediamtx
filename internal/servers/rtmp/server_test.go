package rtmp

import (
	"context"
	"crypto/tls"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
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
	for _, encrypt := range []string{
		"plain",
		"tls",
	} {
		t.Run(encrypt, func(t *testing.T) {
			var serverCertFpath string
			var serverKeyFpath string

			if encrypt == "tls" {
				var err error
				serverCertFpath, err = test.CreateTempFile(test.TLSCertPub)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err = test.CreateTempFile(test.TLSCertKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)
			}

			var strm *stream.Stream
			var reader *stream.Reader
			defer func() {
				strm.RemoveReader(reader)
			}()
			dataReceived := make(chan struct{})
			n := 0

			pathManager := &test.PathManager{
				AddPublisherImpl: func(req defs.PathAddPublisherReq) (defs.Path, *stream.Stream, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "user=myuser&pass=mypass&param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
					require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)

					strm = &stream.Stream{
						WriteQueueSize:     512,
						RTPMaxPayloadSize:  1450,
						Desc:               req.Desc,
						GenerateRTPPackets: true,
						Parent:             test.NilLogger,
					}
					err := strm.Initialize()
					require.NoError(t, err)

					reader = &stream.Reader{Parent: test.NilLogger}

					reader.OnData(
						strm.Desc.Medias[0],
						strm.Desc.Medias[0].Formats[0],
						func(u *unit.Unit) error {
							switch n {
							case 0:
								require.Equal(t, unit.PayloadH264(nil), u.Payload)

							case 1:
								require.Equal(t, unit.PayloadH264{
									test.FormatH264.SPS,
									test.FormatH264.PPS,
									{5, 2, 3, 4},
								}, u.Payload)
								close(dataReceived)

							default:
								t.Errorf("should not happen")
							}
							n++
							return nil
						})

					strm.AddReader(reader)

					return &dummyPath{}, strm, nil
				},
			}

			s := &Server{
				Address:             "127.0.0.1:1939",
				ReadTimeout:         conf.Duration(10 * time.Second),
				WriteTimeout:        conf.Duration(10 * time.Second),
				IsTLS:               encrypt == "tls",
				ServerCert:          serverCertFpath,
				ServerKey:           serverKeyFpath,
				RTSPAddress:         "",
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

			var rawURL string

			if encrypt == "tls" {
				rawURL += "rtmps://"
			} else {
				rawURL += "rtmp://"
			}

			rawURL += "127.0.0.1:1939/teststream?user=myuser&pass=mypass&param=value"

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			conn := &gortmplib.Client{
				URL:       u,
				TLSConfig: &tls.Config{InsecureSkipVerify: true},
				Publish:   true,
			}
			err = conn.Initialize(context.Background())
			require.NoError(t, err)
			defer conn.Close()

			w := &gortmplib.Writer{
				Conn: conn,
				Tracks: []*gortmplib.Track{
					{Codec: &codecs.H264{
						SPS: test.FormatH264.SPS,
						PPS: test.FormatH264.PPS,
					}},
					{Codec: &codecs.MPEG4Audio{
						Config: test.FormatMPEG4Audio.Config,
					}},
				},
			}
			err = w.Initialize()
			require.NoError(t, err)

			err = w.WriteH264(
				w.Tracks[0],
				2*time.Second, 2*time.Second, [][]byte{
					{5, 2, 3, 4},
				})
			require.NoError(t, err)

			<-dataReceived
		})
	}
}

func TestServerRead(t *testing.T) {
	for _, encrypt := range []string{
		"plain",
		"tls",
	} {
		t.Run(encrypt, func(t *testing.T) {
			var serverCertFpath string
			var serverKeyFpath string

			if encrypt == "tls" {
				var err error
				serverCertFpath, err = test.CreateTempFile(test.TLSCertPub)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err = test.CreateTempFile(test.TLSCertKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)
			}
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
					require.Equal(t, "user=myuser&pass=mypass&param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
					require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
					return &dummyPath{}, strm, nil
				},
			}

			s := &Server{
				Address:             "127.0.0.1:1939",
				ReadTimeout:         conf.Duration(10 * time.Second),
				WriteTimeout:        conf.Duration(10 * time.Second),
				IsTLS:               encrypt == "tls",
				ServerCert:          serverCertFpath,
				ServerKey:           serverKeyFpath,
				RTSPAddress:         "",
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

			var rawURL string

			if encrypt == "tls" {
				rawURL += "rtmps://"
			} else {
				rawURL += "rtmp://"
			}

			rawURL += "127.0.0.1:1939/teststream?user=myuser&pass=mypass&param=value"

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			conn := &gortmplib.Client{
				URL:       u,
				TLSConfig: &tls.Config{InsecureSkipVerify: true},
				Publish:   false,
			}
			err = conn.Initialize(context.Background())
			require.NoError(t, err)
			defer conn.Close()

			go func() {
				time.Sleep(500 * time.Millisecond)

				strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					NTP: time.Time{},
					Payload: unit.PayloadH264{
						{5, 2, 3, 4}, // IDR
					},
				})

				strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					NTP: time.Time{},
					PTS: 2 * 90000,
					Payload: unit.PayloadH264{
						{5, 2, 3, 4}, // IDR
					},
				})

				strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					NTP: time.Time{},
					PTS: 3 * 90000,
					Payload: unit.PayloadH264{
						{5, 2, 3, 4}, // IDR
					},
				})
			}()

			r := &gortmplib.Reader{
				Conn: conn,
			}
			err = r.Initialize()
			require.NoError(t, err)

			tracks := r.Tracks()
			require.Len(t, tracks, 1)
			_, ok := tracks[0].Codec.(*codecs.H264)
			require.True(t, ok)

			r.OnDataH264(tracks[0], func(_ time.Duration, _ time.Duration, au [][]byte) {
				require.Equal(t, [][]byte{
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5, 2, 3, 4},
				}, au)
			})

			err = r.Read()
			require.NoError(t, err)
		})
	}
}
