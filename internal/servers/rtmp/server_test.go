package rtmp

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/pires/go-proxyproto"

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
		for _, proxy := range []string{
			"no_proxy",
			"proxy",
		} {
			t.Run(encrypt+"_"+proxy, func(t *testing.T) {
				var serverCertFpath string
				var serverKeyFpath string

				if encrypt == "tls" {
					serverCertFpath = test.CreateTempFile(t, test.TLSCertPub)
					serverKeyFpath = test.CreateTempFile(t, test.TLSCertKey)
				}

				_, ipnet, err := net.ParseCIDR("127.0.0.1/32")
				require.NoError(t, err)
				trustedProxies := conf.IPNetworks{conf.IPNetwork(*ipnet)}

				var strm *stream.Stream
				var reader *stream.Reader
				defer func() {
					strm.RemoveReader(reader)
				}()
				dataReceived := make(chan struct{})
				n := 0

				pathManager := &test.PathManager{
					AddPublisherImpl: func(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
						require.Equal(t, "teststream", req.AccessRequest.Name)
						require.Equal(t, "user=myuser&pass=mypass&param=value", req.AccessRequest.Query)
						require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
						require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)

						strm = &stream.Stream{
							OrigDesc:          req.Desc,
							WriteQueueSize:    512,
							RTPMaxPayloadSize: 1450,
							Parent:            test.NilLogger,
						}
						err2 := strm.Initialize()
						require.NoError(t, err2)

						subStream := &stream.SubStream{
							Stream:        strm,
							UseRTPPackets: false,
						}
						err2 = subStream.Initialize()
						require.NoError(t, err2)

						reader = &stream.Reader{Parent: test.NilLogger}

						reader.OnData(
							strm.OrigDesc.Medias[0],
							strm.OrigDesc.Medias[0].Formats[0],
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

						return &defs.PathAddPublisherRes{
							Path:      &dummyPath{},
							User:      req.AccessRequest.Credentials.User,
							SubStream: subStream,
						}, nil
					},
				}

				s := &Server{
					Address:             "127.0.0.1:1939",
					ReadTimeout:         conf.Duration(10 * time.Second),
					WriteTimeout:        conf.Duration(10 * time.Second),
					Encryption:          encrypt == "tls",
					ServerCert:          serverCertFpath,
					ServerKey:           serverKeyFpath,
					RTSPAddress:         "",
					TrustedProxies:      trustedProxies,
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

				var dialContext func(ctx context.Context, network, address string) (net.Conn, error)
				if proxy == "proxy" {
					dialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
						c, err2 := (&net.Dialer{}).DialContext(ctx, network, address)
						if err2 != nil {
							return nil, err2
						}
						header := &proxyproto.Header{
							Version:           1,
							Command:           proxyproto.PROXY,
							TransportProtocol: proxyproto.TCPv4,
							SourceAddr:        &net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 1234},
							DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1939},
						}
						_, err2 = header.WriteTo(c)
						if err2 != nil {
							return nil, err2
						}
						return c, nil
					}
				}

				conn := &gortmplib.Client{
					URL:         u,
					TLSConfig:   &tls.Config{InsecureSkipVerify: true},
					Publish:     true,
					DialContext: dialContext,
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

				list, err := s.APIConnsList()
				require.NoError(t, err)
				require.Equal(t, &defs.APIRTMPConnList{
					Items: []defs.APIRTMPConn{
						{
							ID:                      list.Items[0].ID,
							Created:                 list.Items[0].Created,
							RemoteAddr:              list.Items[0].RemoteAddr,
							State:                   "publish",
							Path:                    "teststream",
							Query:                   "user=myuser&pass=mypass&param=value",
							User:                    "myuser",
							UserAgent:               list.Items[0].UserAgent,
							InboundBytes:            list.Items[0].InboundBytes,
							OutboundBytes:           list.Items[0].OutboundBytes,
							OutboundFramesDiscarded: list.Items[0].OutboundFramesDiscarded,
							BytesReceived:           list.Items[0].BytesReceived,
							BytesSent:               list.Items[0].BytesSent,
						},
					},
				}, list)
			})
		}
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
				serverCertFpath = test.CreateTempFile(t, test.TLSCertPub)
				serverKeyFpath = test.CreateTempFile(t, test.TLSCertKey)
			}
			desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

			strm := &stream.Stream{
				OrigDesc:          desc,
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

			pathManager := &test.PathManager{
				AddReaderImpl: func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "user=myuser&pass=mypass&param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
					require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
					return &defs.PathAddReaderRes{Path: &dummyPath{}, User: req.AccessRequest.Credentials.User, Stream: strm}, nil
				},
			}

			s := &Server{
				Address:             "127.0.0.1:1939",
				ReadTimeout:         conf.Duration(10 * time.Second),
				WriteTimeout:        conf.Duration(10 * time.Second),
				Encryption:          encrypt == "tls",
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

			strm.WaitForReaders()

			go func() {
				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					NTP: time.Time{},
					Payload: unit.PayloadH264{
						{5, 2, 3, 4}, // IDR
					},
				})

				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					NTP: time.Time{},
					PTS: 2 * 90000,
					Payload: unit.PayloadH264{
						{5, 2, 3, 4}, // IDR
					},
				})

				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
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

			list, err := s.APIConnsList()
			require.NoError(t, err)
			require.Equal(t, &defs.APIRTMPConnList{
				Items: []defs.APIRTMPConn{
					{
						ID:                      list.Items[0].ID,
						Created:                 list.Items[0].Created,
						RemoteAddr:              list.Items[0].RemoteAddr,
						State:                   "read",
						Path:                    "teststream",
						Query:                   "user=myuser&pass=mypass&param=value",
						User:                    "myuser",
						UserAgent:               list.Items[0].UserAgent,
						InboundBytes:            list.Items[0].InboundBytes,
						OutboundBytes:           list.Items[0].OutboundBytes,
						OutboundFramesDiscarded: list.Items[0].OutboundFramesDiscarded,
						BytesReceived:           list.Items[0].BytesReceived,
						BytesSent:               list.Items[0].BytesSent,
					},
				},
			}, list)
		})
	}
}
