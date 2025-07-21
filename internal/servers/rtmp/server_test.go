package rtmp

import (
	"context"
	"crypto/tls"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
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
			streamCreated := make(chan struct{})

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

					close(streamCreated)

					return &dummyPath{}, strm, nil
				},
			}

			s := &Server{
				Address:             "127.0.0.1:1935",
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

			rawURL += "127.0.0.1:1935/teststream?user=myuser&pass=mypass&param=value"

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			conn := &rtmp.Client{
				URL:       u,
				TLSConfig: &tls.Config{InsecureSkipVerify: true},
				Publish:   true,
			}
			err = conn.Initialize(context.Background())
			require.NoError(t, err)
			defer conn.Close()

			w := &rtmp.Writer{
				Conn:       conn,
				VideoTrack: test.FormatH264,
				AudioTrack: test.FormatMPEG4Audio,
			}
			err = w.Initialize()
			require.NoError(t, err)

			err = w.WriteH264(
				2*time.Second, 2*time.Second, [][]byte{
					{5, 2, 3, 4},
				})
			require.NoError(t, err)

			<-streamCreated

			recv := make(chan struct{})

			reader := test.NilLogger

			strm.AddReader(
				reader,
				strm.Desc.Medias[0],
				strm.Desc.Medias[0].Formats[0],
				func(u unit.Unit) error {
					require.Equal(t, [][]byte{
						test.FormatH264.SPS,
						test.FormatH264.PPS,
						{5, 2, 3, 4},
					}, u.(*unit.H264).AU)
					close(recv)
					return nil
				})

			strm.StartReader(reader)
			defer strm.RemoveReader(reader)

			err = w.WriteH264(
				3*time.Second, 3*time.Second, [][]byte{
					{5, 2, 3, 4},
				})
			require.NoError(t, err)

			<-recv
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
				Address:             "127.0.0.1:1935",
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

			rawURL += "127.0.0.1:1935/teststream?user=myuser&pass=mypass&param=value"

			u, err := url.Parse(rawURL)
			require.NoError(t, err)

			conn := &rtmp.Client{
				URL:       u,
				TLSConfig: &tls.Config{InsecureSkipVerify: true},
				Publish:   false,
			}
			err = conn.Initialize(context.Background())
			require.NoError(t, err)
			defer conn.Close()

			go func() {
				strm.WaitRunningReader()

				strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
					},
					AU: [][]byte{
						{5, 2, 3, 4}, // IDR
					},
				})

				strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
						PTS: 2 * 90000,
					},
					AU: [][]byte{
						{5, 2, 3, 4}, // IDR
					},
				})

				strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
						PTS: 3 * 90000,
					},
					AU: [][]byte{
						{5, 2, 3, 4}, // IDR
					},
				})
			}()

			r := &rtmp.Reader{
				Conn: conn,
			}
			err = r.Initialize()
			require.NoError(t, err)

			tracks := r.Tracks()
			require.Equal(t, []format.Format{test.FormatH264}, tracks)

			r.OnDataH264(tracks[0].(*format.H264), func(_ time.Duration, au [][]byte) {
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
