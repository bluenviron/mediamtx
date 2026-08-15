package moq_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/servers/moq"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type serverDummyPath struct{}

func (p *serverDummyPath) Name() string                                  { return "teststream" }
func (p *serverDummyPath) SafeConf() *conf.Path                          { return &conf.Path{} }
func (p *serverDummyPath) ExternalCmdEnv() externalcmd.Environment       { return externalcmd.Environment{} }
func (p *serverDummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {}
func (p *serverDummyPath) RemoveReader(_ defs.PathRemoveReaderReq)       {}

type logRecorder struct {
	mutex sync.Mutex
	logs  []string
}

func (l *logRecorder) Log(_ logger.Level, format string, args ...any) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.logs = append(l.logs, fmt.Sprintf(format, args...))
}

func (l *logRecorder) contains(substr string) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	for _, entry := range l.logs {
		if strings.Contains(entry, substr) {
			return true
		}
	}
	return false
}

func performWTSetup(ctx context.Context, t *testing.T, sx *webtransport.Session, version defs.APIMoQVersion) {
	t.Helper()

	if version == defs.APIMoQVersionDraft16 {
		setupBidi, err := sx.OpenStreamSync(ctx)
		require.NoError(t, err)

		_, err = setupBidi.Write(controlmessage.ClientSetup(controlmessage.Setup{}).Marshal())
		require.NoError(t, err)

		setupMsg, err := controlmessage.Read(setupBidi)
		require.NoError(t, err)
		require.Equal(t, &controlmessage.ServerSetup{}, setupMsg)

		setupBidi.Close() //nolint:errcheck
		return
	}

	setupStream, err := sx.AcceptUniStream(ctx)
	require.NoError(t, err)

	setupMsg, err := controlmessage.Read(setupStream)
	require.NoError(t, err)
	require.Equal(t, &controlmessage.Setup{}, setupMsg)

	clientSetup, err := sx.OpenUniStreamSync(ctx)
	require.NoError(t, err)

	_, err = clientSetup.Write(controlmessage.Setup{}.Marshal())
	require.NoError(t, err)
}

func performNativeQUICSetup(
	ctx context.Context,
	t *testing.T,
	conn *quic.Conn,
	version defs.APIMoQVersion,
	path string,
) {
	t.Helper()

	if version == defs.APIMoQVersionDraft16 {
		setupBidi, err := conn.OpenStreamSync(ctx)
		require.NoError(t, err)

		_, err = setupBidi.Write(controlmessage.ClientSetup(controlmessage.Setup{Path: path}).Marshal())
		require.NoError(t, err)

		setupMsg, err := controlmessage.Read(setupBidi)
		require.NoError(t, err)
		require.Equal(t, &controlmessage.ServerSetup{}, setupMsg)

		setupBidi.Close() //nolint:errcheck
		return
	}

	setupStream, err := conn.AcceptUniStream(ctx)
	require.NoError(t, err)

	setupMsg, err := controlmessage.Read(setupStream)
	require.NoError(t, err)
	require.Equal(t, &controlmessage.Setup{}, setupMsg)

	clientSetup, err := conn.OpenUniStreamSync(ctx)
	require.NoError(t, err)

	_, err = clientSetup.Write(controlmessage.Setup{Path: path}.Marshal())
	require.NoError(t, err)
}

func TestServer(t *testing.T) {
	for _, ca := range []struct {
		name            string
		clientProtocols []string
		expectedVersion defs.APIMoQVersion
	}{
		{
			name:            "draft-16",
			clientProtocols: []string{"moqt-16"},
			expectedVersion: defs.APIMoQVersionDraft16,
		},
		{
			name:            "draft-17",
			clientProtocols: []string{"moqt-17"},
			expectedVersion: defs.APIMoQVersionDraft17,
		},
		{
			name:            "draft-18",
			clientProtocols: []string{"moqt-18"},
			expectedVersion: defs.APIMoQVersionDraft18,
		},
		{
			name:            "draft-19",
			clientProtocols: []string{"moqt-19"},
			expectedVersion: defs.APIMoQVersionDraft19,
		},
		{
			name:            "draft-19-preferred",
			clientProtocols: []string{"moqt-19", "moqt-18"},
			expectedVersion: defs.APIMoQVersionDraft19,
		},
		{
			name:            "highest-preferred",
			clientProtocols: []string{"moqt-16", "moqt-17", "moqt-18", "moqt-19"},
			expectedVersion: defs.APIMoQVersionDraft19,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{test.UniqueMediaH264()}}
			strm := &stream.Stream{
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			pm := &test.PathManager{
				FindPathConfImpl: func(_ defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}}, nil
				},
				AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					return &defs.PathAddReaderRes{Path: &serverDummyPath{}, Stream: strm}, nil
				},
			}

			serverCertFile := test.CreateTempFile(t, test.TLSCertPub)
			serverKeyFile := test.CreateTempFile(t, test.TLSCertKey)

			s := &moq.Server{
				HTTP2Address:   "127.0.0.1:19895",
				HTTP3Address:   "127.0.0.1:19896",
				QUICAddress:    "127.0.0.1:19897",
				ServerCert:     serverCertFile,
				ServerKey:      serverKeyFile,
				AllowOrigins:   []string{"*"},
				TrustedProxies: conf.IPNetworks{},
				ReadTimeout:    conf.Duration(10 * time.Second),
				WriteTimeout:   conf.Duration(10 * time.Second),
				PathManager:    pm,
				Parent:         test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			d := &webtransport.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec
				},
				QUICConfig: &quic.Config{
					EnableDatagrams:                  true,
					EnableStreamResetPartialDelivery: true,
				},
				ApplicationProtocols: ca.clientProtocols,
			}
			defer d.Close() //nolint:errcheck

			res, sx, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream/moq", nil)
			require.NoError(t, err)
			defer sx.CloseWithError(0, "") //nolint:errcheck
			defer res.Body.Close()         //nolint:errcheck

			require.Equal(t, `"`+string(ca.expectedVersion)+`"`, res.Header.Get("WT-Protocol"))

			performWTSetup(ctx, t, sx, ca.expectedVersion)

			catalogBidi, err := sx.OpenStreamSync(ctx)
			require.NoError(t, err)

			_, err = catalogBidi.Write(controlmessage.Subscribe{
				RequestID: 1,
				TrackName: ".catalog",
			}.Marshal())
			require.NoError(t, err)

			catalogOkMsg, err := controlmessage.Read(catalogBidi)
			require.NoError(t, err)
			require.Equal(t, &controlmessage.SubscribeOk{TrackAlias: 1}, catalogOkMsg)

			sessions, err := s.APISessionsList()
			require.NoError(t, err)
			require.Equal(t, 1, len(sessions.Items))
			require.Equal(t, ca.expectedVersion, sessions.Items[0].Version)

			catalogDataStream, err := sx.AcceptUniStream(ctx)
			require.NoError(t, err)

			var catalogSG subgroup.SubGroup
			err = catalogSG.Read(catalogDataStream)
			require.NoError(t, err)

			var cat catalog.Catalog
			err = json.Unmarshal(catalogSG.Objects[0].Payload, &cat)
			require.NoError(t, err)

			require.Equal(t, catalog.Catalog{
				Version: 1,
				Tracks: []catalog.Track{{
					Name:      "0",
					Packaging: "loc",
					IsLive:    true,
					Codec:     "avc3.640028",
				}},
			}, cat)

			trackBidi, err := sx.OpenStreamSync(ctx)
			require.NoError(t, err)

			_, err = trackBidi.Write(controlmessage.Subscribe{
				RequestID: 2,
				TrackName: "0",
			}.Marshal())
			require.NoError(t, err)

			trackOkMsg, err := controlmessage.Read(trackBidi)
			require.NoError(t, err)
			require.Equal(t, &controlmessage.SubscribeOk{TrackAlias: 2}, trackOkMsg)

			trackBidi2, err := sx.OpenStreamSync(ctx)
			require.NoError(t, err)

			_, err = trackBidi2.Write(controlmessage.Subscribe{
				RequestID: 3,
				TrackName: "0",
			}.Marshal())
			require.NoError(t, err)

			trackOkMsg2, err := controlmessage.Read(trackBidi2)
			require.NoError(t, err)
			require.Equal(t, &controlmessage.SubscribeOk{TrackAlias: 3}, trackOkMsg2)

			go func() {
				time.Sleep(200 * time.Millisecond)
				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					PTS:     0,
					Payload: unit.PayloadH264{{5, 1}},
				})
			}()

			frameStream, err := sx.AcceptUniStream(ctx)
			require.NoError(t, err)

			var frameSG subgroup.SubGroup
			err = frameSG.Read(frameStream)
			require.NoError(t, err)

			frameStream2, err := sx.AcceptUniStream(ctx)
			require.NoError(t, err)

			var frameSG2 subgroup.SubGroup
			err = frameSG2.Read(frameStream2)
			require.NoError(t, err)

			expectedPayload, err2 := h264.AVCC([][]byte{test.FormatH264.SPS, test.FormatH264.PPS, {5, 1}}).Marshal()
			require.NoError(t, err2)
			require.Equal(t, expectedPayload, frameSG.Objects[0].Payload)
			require.Equal(t, expectedPayload, frameSG2.Objects[0].Payload)
			require.ElementsMatch(t, []uint64{2, 3}, []uint64{frameSG.Header.TrackAlias, frameSG2.Header.TrackAlias})

			trackBidi.Close() //nolint:errcheck
			time.Sleep(100 * time.Millisecond)

			go func() {
				time.Sleep(200 * time.Millisecond)
				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					PTS:     1,
					Payload: unit.PayloadH264{{5, 2}},
				})
			}()

			frameStream3, err := sx.AcceptUniStream(ctx)
			require.NoError(t, err)

			var frameSG3 subgroup.SubGroup
			err = frameSG3.Read(frameStream3)
			require.NoError(t, err)

			expectedPayload2, err2 := h264.AVCC([][]byte{test.FormatH264.SPS, test.FormatH264.PPS, {5, 2}}).Marshal()
			require.NoError(t, err2)
			require.Equal(t, expectedPayload2, frameSG3.Objects[0].Payload)
			require.Equal(t, uint64(3), frameSG3.Header.TrackAlias)
			trackBidi2.Close() //nolint:errcheck
		})
	}
}

func TestServerNativeQUICSubscribe(t *testing.T) {
	for _, ca := range []struct {
		name    string
		version defs.APIMoQVersion
	}{
		{
			name:    "draft-16",
			version: defs.APIMoQVersionDraft16,
		},
		{
			name:    "draft-19",
			version: defs.APIMoQVersionDraft19,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{test.UniqueMediaH264()}}
			strm := &stream.Stream{
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			pm := &test.PathManager{
				FindPathConfImpl: func(_ defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}}, nil
				},
				AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					return &defs.PathAddReaderRes{Path: &serverDummyPath{}, Stream: strm}, nil
				},
			}

			serverCertFile := test.CreateTempFile(t, test.TLSCertPub)
			serverKeyFile := test.CreateTempFile(t, test.TLSCertKey)

			s := &moq.Server{
				HTTP2Address:   "127.0.0.1:19895",
				HTTP3Address:   "127.0.0.1:19896",
				QUICAddress:    "127.0.0.1:19897",
				ServerCert:     serverCertFile,
				ServerKey:      serverKeyFile,
				AllowOrigins:   []string{"*"},
				TrustedProxies: conf.IPNetworks{},
				ReadTimeout:    conf.Duration(10 * time.Second),
				WriteTimeout:   conf.Duration(10 * time.Second),
				PathManager:    pm,
				Parent:         test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			conn, err := quic.DialAddr(ctx, "127.0.0.1:19897", &tls.Config{ //nolint:gosec
				InsecureSkipVerify: true,
				NextProtos:         []string{string(ca.version)},
			}, &quic.Config{EnableDatagrams: true})
			require.NoError(t, err)
			defer conn.CloseWithError(0, "") //nolint:errcheck

			performNativeQUICSetup(ctx, t, conn, ca.version, "/teststream")

			catalogBidi, err := conn.OpenStreamSync(ctx)
			require.NoError(t, err)

			_, err = catalogBidi.Write(controlmessage.Subscribe{
				RequestID: 1,
				TrackName: ".catalog",
			}.Marshal())
			require.NoError(t, err)

			catalogOkMsg, err := controlmessage.Read(catalogBidi)
			require.NoError(t, err)
			require.Equal(t, &controlmessage.SubscribeOk{TrackAlias: 1}, catalogOkMsg)

			sessions, err := s.APISessionsList()
			require.NoError(t, err)
			require.Equal(t, 1, len(sessions.Items))
			require.Equal(t, ca.version, sessions.Items[0].Version)
			require.Equal(t, defs.APIMoQSessionTransportQUIC, sessions.Items[0].Transport)

			catalogDataStream, err := conn.AcceptUniStream(ctx)
			require.NoError(t, err)

			var catalogSG subgroup.SubGroup
			err = catalogSG.Read(catalogDataStream)
			require.NoError(t, err)

			var cat catalog.Catalog
			err = json.Unmarshal(catalogSG.Objects[0].Payload, &cat)
			require.NoError(t, err)

			require.Equal(t, catalog.Catalog{
				Version: 1,
				Tracks: []catalog.Track{{
					Name:      "0",
					Packaging: "loc",
					IsLive:    true,
					Codec:     "avc3.640028",
				}},
			}, cat)
		})
	}
}

func TestServerAuthError(t *testing.T) {
	serverCertFile := test.CreateTempFile(t, test.TLSCertPub)
	serverKeyFile := test.CreateTempFile(t, test.TLSCertKey)

	for _, ca := range []string{
		"read page",
		"publish page",
		"subscribe",
		"publish",
	} {
		t.Run(ca, func(t *testing.T) {
			switch ca {
			case "read page", "publish page":
				pm := &test.PathManager{
					FindPathConfImpl: func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
						if req.AccessRequest.Credentials.User == "" && req.AccessRequest.Credentials.Pass == "" {
							return nil, &auth.Error{AskCredentials: true, Wrapped: fmt.Errorf("auth error")}
						}

						return nil, &auth.Error{Wrapped: fmt.Errorf("auth error")}
					},
				}
				s := &moq.Server{
					HTTP2Address:   "127.0.0.1:19895",
					HTTP3Address:   "127.0.0.1:19896",
					QUICAddress:    "127.0.0.1:19897",
					ServerCert:     serverCertFile,
					ServerKey:      serverKeyFile,
					AllowOrigins:   []string{"*"},
					TrustedProxies: conf.IPNetworks{},
					ReadTimeout:    conf.Duration(10 * time.Second),
					WriteTimeout:   conf.Duration(10 * time.Second),
					PathManager:    pm,
					Parent:         test.NilLogger,
				}
				err := s.Initialize()
				require.NoError(t, err)
				defer s.Close()

				tr := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				}
				defer tr.CloseIdleConnections()
				hc := &http.Client{Transport: tr}

				var req *http.Request

				switch ca {
				case "read page":
					req, err = http.NewRequest(http.MethodGet, "https://127.0.0.1:19895/stream/", nil)

				case "publish page":
					req, err = http.NewRequest(http.MethodGet, "https://127.0.0.1:19895/stream/publish", nil)
				}
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusUnauthorized, res.StatusCode)
				require.Equal(t, `Basic realm="mediamtx"`, res.Header.Get("WWW-Authenticate"))

				switch ca {
				case "read page":
					req, err = http.NewRequest(http.MethodGet, "https://myuser:mypass@127.0.0.1:19895/stream/", nil)

				case "publish page":
					req, err = http.NewRequest(http.MethodGet, "https://myuser:mypass@127.0.0.1:19895/stream/publish", nil)
				}
				require.NoError(t, err)

				res, err = hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusUnauthorized, res.StatusCode)
				require.Equal(t, ``, res.Header.Get("WWW-Authenticate"))

			case "subscribe", "publish":
				pm := &test.PathManager{}

				switch ca {
				case "subscribe":
					pm.AddReaderImpl = func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
						return nil, &auth.Error{Wrapped: fmt.Errorf("auth error")}
					}

				case "publish":
					pm.AddPublisherImpl = func(_ defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
						return nil, &auth.Error{Wrapped: fmt.Errorf("auth error")}
					}
				}

				s := &moq.Server{
					HTTP2Address:   "127.0.0.1:19895",
					HTTP3Address:   "127.0.0.1:19896",
					QUICAddress:    "127.0.0.1:19897",
					ServerCert:     serverCertFile,
					ServerKey:      serverKeyFile,
					AllowOrigins:   []string{"*"},
					TrustedProxies: conf.IPNetworks{},
					ReadTimeout:    conf.Duration(10 * time.Second),
					WriteTimeout:   conf.Duration(10 * time.Second),
					PathManager:    pm,
					Parent:         test.NilLogger,
				}
				err := s.Initialize()
				require.NoError(t, err)
				defer s.Close()

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				d := &webtransport.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
					QUICConfig: &quic.Config{
						EnableDatagrams:                  true,
						EnableStreamResetPartialDelivery: true,
					},
					ApplicationProtocols: []string{"moqt-19"},
				}
				defer d.Close() //nolint:errcheck

				res, sx, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream/moq", nil)
				require.NoError(t, err)
				defer sx.CloseWithError(0, "") //nolint:errcheck
				defer res.Body.Close()         //nolint:errcheck

				performWTSetup(ctx, t, sx, defs.APIMoQVersionDraft19)

				switch ca {
				case "subscribe":
					var catalogBidi *webtransport.Stream
					catalogBidi, err = sx.OpenStreamSync(ctx)
					require.NoError(t, err)

					_, err = catalogBidi.Write(controlmessage.Subscribe{
						RequestID: 1,
						TrackName: ".catalog",
						Parameters: parameter.Parameters{
							&parameter.AuthorizationToken{
								AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
								TokenType:  1,
								TokenValue: []byte("Basic bXl1c2VyOm15cGFzcw=="),
							},
						},
					}.Marshal())
					require.NoError(t, err)

					var msg controlmessage.Message
					msg, err = controlmessage.Read(catalogBidi)
					require.NoError(t, err)

					require.Equal(t, &controlmessage.RequestError{
						Code:   controlmessage.RequestErrorCodeUnauthorized,
						Reason: "failed to authenticate: auth error",
					}, msg)

					catalogBidi.Close() //nolint:errcheck

				case "publish":
					var catalogData *webtransport.SendStream
					catalogData, err = sx.OpenUniStreamSync(ctx)
					require.NoError(t, err)

					var cat []byte
					cat, err = json.Marshal(catalog.Catalog{Version: 1, Tracks: []catalog.Track{{
						Name:      "0",
						Packaging: "loc",
						IsLive:    true,
						Codec:     "avc3.640028",
					}}})
					require.NoError(t, err)

					_, err = catalogData.Write((&subgroup.SubGroup{
						Header:  subgroup.Header{FirstObject: true, TrackAlias: 0, GroupID: 0},
						Objects: []subgroup.Object{{Payload: cat}},
					}).Marshal())
					require.NoError(t, err)
					catalogData.Close() //nolint:errcheck

					var catalogBidi *webtransport.Stream
					catalogBidi, err = sx.OpenStreamSync(ctx)
					require.NoError(t, err)

					_, err = catalogBidi.Write(controlmessage.Publish{
						RequestID:  1,
						TrackName:  ".catalog",
						TrackAlias: 0,
						Parameters: parameter.Parameters{
							&parameter.AuthorizationToken{
								AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
								TokenType:  1,
								TokenValue: []byte("Basic bXl1c2VyOm15cGFzcw=="),
							},
						},
					}.Marshal())
					require.NoError(t, err)

					var msg controlmessage.Message
					msg, err = controlmessage.Read(catalogBidi)
					require.NoError(t, err)

					require.Equal(t, &controlmessage.RequestError{
						Code:   controlmessage.RequestErrorCodeUnauthorized,
						Reason: "failed to authenticate: auth error",
					}, msg)

					catalogBidi.Close() //nolint:errcheck
				}
			}
		})
	}
}

func TestServerErrorUnsupportedVersion(t *testing.T) {
	serverCertFile := test.CreateTempFile(t, test.TLSCertPub)
	serverKeyFile := test.CreateTempFile(t, test.TLSCertKey)

	s := &moq.Server{
		HTTP2Address:   "127.0.0.1:19895",
		HTTP3Address:   "127.0.0.1:19896",
		QUICAddress:    "127.0.0.1:19897",
		ServerCert:     serverCertFile,
		ServerKey:      serverKeyFile,
		AllowOrigins:   []string{"*"},
		TrustedProxies: conf.IPNetworks{},
		ReadTimeout:    conf.Duration(10 * time.Second),
		WriteTimeout:   conf.Duration(10 * time.Second),
		PathManager:    &test.PathManager{},
		Parent:         test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d := &webtransport.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
		ApplicationProtocols: []string{"moqt-20"},
	}
	defer d.Close() //nolint:errcheck

	res, _, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream/moq", nil)
	require.Error(t, err)
	defer res.Body.Close()
}

func TestServerErrorTooManyTracks(t *testing.T) {
	serverCertFile := test.CreateTempFile(t, test.TLSCertPub)
	serverKeyFile := test.CreateTempFile(t, test.TLSCertKey)

	var addPublisherCalled atomic.Bool
	pm := &test.PathManager{
		AddPublisherImpl: func(_ defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
			addPublisherCalled.Store(true)
			return nil, nil
		},
	}

	parent := &logRecorder{}
	s := &moq.Server{
		HTTP2Address:   "127.0.0.1:19895",
		HTTP3Address:   "127.0.0.1:19896",
		QUICAddress:    "127.0.0.1:19897",
		ServerCert:     serverCertFile,
		ServerKey:      serverKeyFile,
		AllowOrigins:   []string{"*"},
		TrustedProxies: conf.IPNetworks{},
		ReadTimeout:    conf.Duration(10 * time.Second),
		WriteTimeout:   conf.Duration(10 * time.Second),
		PathManager:    pm,
		Parent:         parent,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d := &webtransport.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
		ApplicationProtocols: []string{"moqt-19"},
	}
	defer d.Close() //nolint:errcheck

	res, sx, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream/moq", nil)
	require.NoError(t, err)
	defer sx.CloseWithError(0, "") //nolint:errcheck
	defer res.Body.Close()         //nolint:errcheck

	performWTSetup(ctx, t, sx, defs.APIMoQVersionDraft19)

	tracks := make([]catalog.Track, 51)
	for i := range tracks {
		tracks[i] = catalog.Track{
			Name:      fmt.Sprintf("%d", i),
			Packaging: "loc",
			IsLive:    true,
			Codec:     "avc3.640028",
		}
	}

	cat, err := json.Marshal(catalog.Catalog{Version: 1, Tracks: tracks})
	require.NoError(t, err)

	catalogData, err := sx.OpenUniStreamSync(ctx)
	require.NoError(t, err)

	_, err = catalogData.Write((&subgroup.SubGroup{
		Header:  subgroup.Header{FirstObject: true, TrackAlias: 0, GroupID: 0},
		Objects: []subgroup.Object{{Payload: cat}},
	}).Marshal())
	require.NoError(t, err)
	catalogData.Close() //nolint:errcheck

	catalogBidi, err := sx.OpenStreamSync(ctx)
	require.NoError(t, err)
	defer catalogBidi.Close() //nolint:errcheck

	_, err = catalogBidi.Write(controlmessage.Publish{
		RequestID:  1,
		TrackName:  ".catalog",
		TrackAlias: 0,
	}.Marshal())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return parent.contains("too many catalog tracks: 51")
	}, time.Second, 50*time.Millisecond)
	require.False(t, addPublisherCalled.Load())
}
