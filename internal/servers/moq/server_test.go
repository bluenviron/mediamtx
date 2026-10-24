package moq_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	protomoq "github.com/bluenviron/mediamtx/internal/protocols/moq"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
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

func (l *logRecorder) OnFatalError(context.Context, error) {
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

func performWTSetup(ctx context.Context, t *testing.T, sx *webtransport.Session, versio version.Version) {
	t.Helper()

	if versio == version.Draft16 {
		setupBidi, err := sx.OpenStreamSync(ctx)
		require.NoError(t, err)

		_, err = setupBidi.Write(controlmessage.ClientSetup(controlmessage.Setup{}).Marshal(version.Draft16))
		require.NoError(t, err)

		setupMsg, err := controlmessage.Read(setupBidi, version.Draft16)
		require.NoError(t, err)
		require.Equal(t, &controlmessage.ServerSetup{}, setupMsg)

		setupBidi.Close() //nolint:errcheck
		return
	}

	setupStream, err := sx.AcceptUniStream(ctx)
	require.NoError(t, err)

	setupMsg, err := controlmessage.Read(setupStream, versio)
	require.NoError(t, err)
	require.Equal(t, &controlmessage.Setup{}, setupMsg)

	clientSetup, err := sx.OpenUniStreamSync(ctx)
	require.NoError(t, err)

	_, err = clientSetup.Write(controlmessage.Setup{}.Marshal(versio))
	require.NoError(t, err)
}

func TestServerReadExpiration(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{test.UniqueMediaH264()}}
	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	pm := &test.PathManager{
		AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
			expiresAt := time.Now().Add(3 * time.Second)
			return &defs.PathAddReaderRes{
				Path:    &serverDummyPath{},
				Stream:  strm,
				AuthRes: &auth.Result{User: "somebody", ExpiresAt: &expiresAt},
			}, nil
		},
	}
	s := &moq.Server{
		HTTP2Address: "127.0.0.1:19895",
		HTTP3Address: "127.0.0.1:19896",
		QUICAddress:  "127.0.0.1:19897",
		ServerCert:   test.CreateTempFile(t, test.TLSCertPub),
		ServerKey:    test.CreateTempFile(t, test.TLSCertKey),
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		PathManager:  pm,
		Parent:       test.NilLogger,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	u, err := url.Parse("moqt://127.0.0.1:19896/teststream")
	require.NoError(t, err)
	client := &protomoq.Client{
		URL:             u,
		Transport:       conf.MoQTransportWebTransport,
		TLSConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		ClientProtocols: []version.Version{version.Draft19},
		Log:             test.NilLogger,
	}
	require.NoError(t, client.Initialize(ctx))
	defer client.Close() //nolint:errcheck
	require.NoError(t, client.Subscribe(ctx, "catalog", func(_ *subgroup.SubGroup) error { return nil }))

	require.Eventually(t, func() bool {
		list, listErr := s.APISessionsList()
		return listErr == nil && len(list.Items) == 1 && list.Items[0].State == defs.APIMoQSessionStateRead
	}, time.Second, 10*time.Millisecond)
	list, err := s.APISessionsList()
	require.NoError(t, err)
	require.Equal(t, "somebody", list.Items[0].User)
	require.NotNil(t, list.Items[0].ExpiresAt)
	require.Eventually(t, func() bool {
		currentList, listErr := s.APISessionsList()
		return listErr == nil && len(currentList.Items) == 0
	}, 5*time.Second, 20*time.Millisecond)
}

func TestServerPublishExpiration(t *testing.T) {
	strm := &stream.Stream{
		OrigDesc:          &description.Session{Medias: []*description.Media{test.UniqueMediaH264()}},
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	subStream := &stream.SubStream{Stream: strm, UseRTPPackets: false}
	require.NoError(t, subStream.Initialize())

	pm := &test.PathManager{
		AddPublisherImpl: func(_ defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
			expiresAt := time.Now().Add(2 * time.Second)
			return &defs.PathAddPublisherRes{
				Path:      &serverDummyPath{},
				SubStream: subStream,
				AuthRes:   &auth.Result{ExpiresAt: &expiresAt},
			}, nil
		},
	}
	logs := &logRecorder{}
	s := &moq.Server{
		HTTP2Address: "127.0.0.1:19895",
		HTTP3Address: "127.0.0.1:19896",
		QUICAddress:  "127.0.0.1:19897",
		ServerCert:   test.CreateTempFile(t, test.TLSCertPub),
		ServerKey:    test.CreateTempFile(t, test.TLSCertKey),
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		PathManager:  pm,
		Parent:       logs,
	}
	require.NoError(t, s.Initialize())
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	u, err := url.Parse("moqt://127.0.0.1:19897/teststream")
	require.NoError(t, err)
	client := &protomoq.Client{
		URL:       u,
		Transport: conf.MoQTransportQUIC,
		TLSConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		Log:       test.NilLogger,
	}
	require.NoError(t, client.Initialize(ctx))
	defer client.Close() //nolint:errcheck

	require.True(t, logs.contains("[s->c] SETUP path= authority="))

	cat, err := json.Marshal(catalog.Catalog{Version: 1, Tracks: []catalog.Track{{
		Name:      "0",
		Packaging: "loc",
		IsLive:    true,
		Codec:     "avc3.640028",
	}}})
	require.NoError(t, err)
	require.NoError(t, client.WriteSubGroup(ctx, 0, 0, nil, cat))
	require.NoError(t, client.Publish(ctx, "catalog", 0, nil))
	require.True(t, logs.contains("[s->c] REQUEST_OK"))

	require.Eventually(t, func() bool {
		list, listErr := s.APISessionsList()
		return listErr == nil && len(list.Items) == 1 && list.Items[0].State == defs.APIMoQSessionStatePublish
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return logs.contains("closed: session expired")
	}, 5*time.Second, 20*time.Millisecond)
}

func TestServer(t *testing.T) {
	for _, ca := range []struct {
		name            string
		clientProtocols []version.Version
		expectedVersion version.Version
		catalogName     string
	}{
		{
			name:            "draft-16",
			clientProtocols: []version.Version{version.Draft16},
			expectedVersion: version.Draft16,
		},
		{
			name:            "draft-17",
			clientProtocols: []version.Version{version.Draft17},
			expectedVersion: version.Draft17,
		},
		{
			name:            "draft-18",
			clientProtocols: []version.Version{version.Draft18},
			expectedVersion: version.Draft18,
		},
		{
			name:            "draft-19",
			clientProtocols: []version.Version{version.Draft19},
			expectedVersion: version.Draft19,
		},
		{
			name:            "draft-20",
			clientProtocols: []version.Version{version.Draft20},
			expectedVersion: version.Draft20,
		},
		{
			name:            "draft-21",
			clientProtocols: []version.Version{version.Draft21},
			expectedVersion: version.Draft21,
		},
		{
			name:            "draft-22",
			clientProtocols: []version.Version{version.Draft22},
			expectedVersion: version.Draft22,
		},
		{
			name:            "draft-19-preferred",
			clientProtocols: []version.Version{version.Draft19, version.Draft18},
			expectedVersion: version.Draft19,
		},
		{
			name:            "highest-preferred",
			clientProtocols: []version.Version{version.Draft16, version.Draft17, version.Draft18, version.Draft19},
			expectedVersion: version.Draft19,
		},
		{
			name:            "legacy-catalog",
			clientProtocols: []version.Version{version.Draft19},
			expectedVersion: version.Draft19,
			catalogName:     ".catalog",
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
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}, AuthRes: &auth.Result{}}, nil
				},
				AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					return &defs.PathAddReaderRes{
						Path:    &serverDummyPath{},
						Stream:  strm,
						AuthRes: &auth.Result{User: "reader"},
					}, nil
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

			u, err := url.Parse("moqt://127.0.0.1:19896/teststream")
			require.NoError(t, err)

			client := &protomoq.Client{
				URL:             u,
				Transport:       conf.MoQTransportWebTransport,
				TLSConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				ClientProtocols: ca.clientProtocols,
				Log:             test.NilLogger,
			}
			err = client.Initialize(ctx)
			require.NoError(t, err)
			defer client.Close() //nolint:errcheck

			require.Equal(t, ca.expectedVersion, client.Version())

			sessions, err := s.APISessionsList()
			require.NoError(t, err)
			require.Len(t, sessions.Items, 1)
			require.Equal(t, defs.APIMoQVersion(ca.expectedVersion.String()), sessions.Items[0].Version)
			require.Equal(t, defs.APIMoQSessionTransportWebTransport, sessions.Items[0].Transport)

			received := make(chan *subgroup.SubGroup, 1)
			catalogName := ca.catalogName
			if catalogName == "" {
				catalogName = "catalog"
			}
			err = client.Subscribe(ctx, catalogName, func(sg *subgroup.SubGroup) error {
				received <- sg
				return nil
			})
			require.NoError(t, err)

			select {
			case catalogSG := <-received:
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

			case <-ctx.Done():
				t.Fatal("timeout waiting for catalog")
			}

			sessions, err = s.APISessionsList()
			require.NoError(t, err)
			require.Len(t, sessions.Items, 1)
			require.Equal(t, "reader", sessions.Items[0].User)
		})
	}
}

func TestServerWebTransportSubscriptionControlStreamLifetime(t *testing.T) {
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
			return &defs.PathFindPathConfRes{Conf: &conf.Path{}, AuthRes: &auth.Result{}}, nil
		},
		AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
			return &defs.PathAddReaderRes{Path: &serverDummyPath{}, Stream: strm, AuthRes: &auth.Result{}}, nil
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
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
		ApplicationProtocols: []string{"moqt-19"},
	}
	defer d.Close() //nolint:errcheck

	res, sx, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream", nil)
	require.NoError(t, err)
	defer sx.CloseWithError(0, "") //nolint:errcheck
	defer res.Body.Close()         //nolint:errcheck

	performWTSetup(ctx, t, sx, version.Draft19)

	subscribe := func(requestID uint64, trackName string) *webtransport.Stream {
		bidi, openErr := sx.OpenStreamSync(ctx)
		require.NoError(t, openErr)

		_, writeErr := bidi.Write(controlmessage.Subscribe{
			RequestID: requestID,
			TrackName: trackName,
		}.Marshal(version.Draft19))
		require.NoError(t, writeErr)

		msg, readErr := controlmessage.Read(bidi, version.Draft19)
		require.NoError(t, readErr)
		require.IsType(t, &controlmessage.SubscribeOk{}, msg)

		return bidi
	}

	catalogBidi := subscribe(1, "catalog")
	defer catalogBidi.Close() //nolint:errcheck

	catalogDataStream, err := sx.AcceptUniStream(ctx)
	require.NoError(t, err)

	var catalogSG subgroup.SubGroup
	err = catalogSG.Read(catalogDataStream)
	require.NoError(t, err)

	var cat catalog.Catalog
	err = json.Unmarshal(catalogSG.Objects[0].Payload, &cat)
	require.NoError(t, err)
	require.Len(t, cat.Tracks, 1)

	trackBidi := subscribe(2, "0")
	trackBidi2 := subscribe(3, "0")

	go func() {
		time.Sleep(200 * time.Millisecond)
		subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS:     0,
			Payload: unit.PayloadH264{{5, 1}},
		})
	}()

	readSubGroup := func() subgroup.SubGroup {
		frameStream, acceptErr := sx.AcceptUniStream(ctx)
		require.NoError(t, acceptErr)

		var frameSG subgroup.SubGroup
		readErr := frameSG.Read(frameStream)
		require.NoError(t, readErr)
		return frameSG
	}

	frameSG := readSubGroup()
	frameSG2 := readSubGroup()

	expectedPayload, err := h264.AVCC([][]byte{test.FormatH264.SPS, test.FormatH264.PPS, {5, 1}}).Marshal()
	require.NoError(t, err)
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

	frameSG3 := readSubGroup()

	expectedPayload2, err := h264.AVCC([][]byte{test.FormatH264.SPS, test.FormatH264.PPS, {5, 2}}).Marshal()
	require.NoError(t, err)
	require.Equal(t, expectedPayload2, frameSG3.Objects[0].Payload)
	require.Equal(t, uint64(3), frameSG3.Header.TrackAlias)
	trackBidi2.Close() //nolint:errcheck
}

func TestServerNativeQUICSubscribe(t *testing.T) {
	for _, ca := range []struct {
		name            string
		clientProtocols []version.Version
		expectedVersion version.Version
		catalogName     string
	}{
		{
			name:            "draft-16",
			clientProtocols: []version.Version{version.Draft16},
			expectedVersion: version.Draft16,
		},
		{
			name:            "draft-20",
			clientProtocols: []version.Version{version.Draft20},
			expectedVersion: version.Draft20,
		},
		{
			name:            "draft-21",
			clientProtocols: []version.Version{version.Draft21},
			expectedVersion: version.Draft21,
		},
		{
			name:            "draft-22",
			clientProtocols: []version.Version{version.Draft22},
			expectedVersion: version.Draft22,
		},
		{
			name:            "default",
			expectedVersion: version.Draft22,
		},
		{
			name:            "legacy-catalog",
			clientProtocols: []version.Version{version.Draft19},
			expectedVersion: version.Draft19,
			catalogName:     ".catalog",
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
					return &defs.PathFindPathConfRes{Conf: &conf.Path{}, AuthRes: &auth.Result{}}, nil
				},
				AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					return &defs.PathAddReaderRes{Path: &serverDummyPath{}, Stream: strm, AuthRes: &auth.Result{}}, nil
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

			u, err := url.Parse("moqt://127.0.0.1:19897/teststream")
			require.NoError(t, err)

			client := &protomoq.Client{
				URL:             u,
				Transport:       conf.MoQTransportQUIC,
				TLSConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				ClientProtocols: ca.clientProtocols,
				Log:             test.NilLogger,
			}
			err = client.Initialize(ctx)
			require.NoError(t, err)
			defer client.Close() //nolint:errcheck

			require.Equal(t, ca.expectedVersion, client.Version())

			received := make(chan *subgroup.SubGroup, 1)
			catalogName := ca.catalogName
			if catalogName == "" {
				catalogName = "catalog"
			}
			err = client.Subscribe(ctx, catalogName, func(sg *subgroup.SubGroup) error {
				received <- sg
				return nil
			})
			require.NoError(t, err)

			sessions, err := s.APISessionsList()
			require.NoError(t, err)
			require.Len(t, sessions.Items, 1)
			require.Equal(t, defs.APIMoQVersion(ca.expectedVersion.String()), sessions.Items[0].Version)
			require.Equal(t, defs.APIMoQSessionTransportQUIC, sessions.Items[0].Transport)

			select {
			case catalogSG := <-received:
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

			case <-ctx.Done():
				t.Fatal("timeout waiting for catalog")
			}
		})
	}
}

func TestServerPublishCatalogNames(t *testing.T) {
	for _, catalogName := range []string{"catalog", ".catalog"} {
		t.Run(catalogName, func(t *testing.T) {
			serverCertFile := test.CreateTempFile(t, test.TLSCertPub)
			serverKeyFile := test.CreateTempFile(t, test.TLSCertKey)

			strm := &stream.Stream{
				OrigDesc:          &description.Session{Medias: []*description.Media{test.UniqueMediaH264()}},
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &stream.SubStream{Stream: strm, UseRTPPackets: false}
			err = subStream.Initialize()
			require.NoError(t, err)

			published := make(chan *description.Session, 1)
			pm := &test.PathManager{
				AddPublisherImpl: func(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
					published <- req.Desc
					return &defs.PathAddPublisherRes{
						Path:      &serverDummyPath{},
						SubStream: subStream,
						AuthRes:   &auth.Result{User: "publisher"},
					}, nil
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
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			u, err := url.Parse("moqt://127.0.0.1:19897/teststream")
			require.NoError(t, err)

			client := &protomoq.Client{
				URL:       u,
				Transport: conf.MoQTransportQUIC,
				TLSConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				Log:       test.NilLogger,
			}
			err = client.Initialize(ctx)
			require.NoError(t, err)
			defer client.Close() //nolint:errcheck

			cat, err := json.Marshal(catalog.Catalog{Version: 1, Tracks: []catalog.Track{{
				Name:      "0",
				Packaging: "loc",
				IsLive:    true,
				Codec:     "avc3.640028",
			}}})
			require.NoError(t, err)

			err = client.WriteSubGroup(ctx, 0, 0, nil, cat)
			require.NoError(t, err)
			err = client.Publish(ctx, catalogName, 0, nil)
			require.NoError(t, err)

			select {
			case desc := <-published:
				require.Len(t, desc.Medias, 1)
			case <-ctx.Done():
				t.Fatal("timeout waiting for catalog publication")
			}

			require.Eventually(t, func() bool {
				sessions, listErr := s.APISessionsList()
				return listErr == nil && len(sessions.Items) == 1 && sessions.Items[0].User == "publisher"
			}, time.Second, 10*time.Millisecond)
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

				res, sx, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream", nil)
				require.NoError(t, err)
				defer sx.CloseWithError(0, "") //nolint:errcheck
				defer res.Body.Close()         //nolint:errcheck

				performWTSetup(ctx, t, sx, version.Draft19)

				switch ca {
				case "subscribe":
					var catalogBidi *webtransport.Stream
					catalogBidi, err = sx.OpenStreamSync(ctx)
					require.NoError(t, err)

					_, err = catalogBidi.Write(controlmessage.Subscribe{
						RequestID: 1,
						TrackName: "catalog",
						Parameters: parameter.Parameters{
							&parameter.AuthorizationToken{
								AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
								TokenType:  1,
								TokenValue: []byte("Basic bXl1c2VyOm15cGFzcw=="),
							},
						},
					}.Marshal(version.Draft19))
					require.NoError(t, err)

					var msg controlmessage.Message
					msg, err = controlmessage.Read(catalogBidi, version.Draft19)
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
						Header:  subgroup.Header{IsFirstObject: true, TrackAlias: 0, GroupID: 0},
						Objects: []subgroup.Object{{Payload: cat}},
					}).Marshal())
					require.NoError(t, err)
					catalogData.Close() //nolint:errcheck

					var catalogBidi *webtransport.Stream
					catalogBidi, err = sx.OpenStreamSync(ctx)
					require.NoError(t, err)

					_, err = catalogBidi.Write(controlmessage.Publish{
						RequestID:  1,
						TrackName:  "catalog",
						TrackAlias: 0,
						Parameters: parameter.Parameters{
							&parameter.AuthorizationToken{
								AliasType:  parameter.AuthorizationTokenAliasTypeUseValue,
								TokenType:  1,
								TokenValue: []byte("Basic bXl1c2VyOm15cGFzcw=="),
							},
						},
					}.Marshal(version.Draft19))
					require.NoError(t, err)

					var msg controlmessage.Message
					msg, err = controlmessage.Read(catalogBidi, version.Draft19)
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
		ApplicationProtocols: []string{"moqt-23"},
	}
	defer d.Close() //nolint:errcheck

	res, _, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream", nil)
	require.Error(t, err)
	defer res.Body.Close()
}

func TestServerNonDefaultParametersReachReader(t *testing.T) {
	for _, ca := range []struct {
		version version.Version
		filter  []byte
	}{
		{version.Draft20, []byte{0x21, 0x02, 0x00, 0x00}},
		{version.Draft21, []byte{0x21, 0x02, 0x00, 0x00}},
		{version.Draft22, []byte{0x21, 0x05}},
		{version.Draft22, []byte{0x23, 0x00}},
		{version.Draft22, []byte{0x35, 0x01}},
		{version.Draft22, []byte{0x23, 0x02, 0x01, 0x02}},
	} {
		t.Run(ca.version.String()+fmt.Sprintf("-%x", ca.filter), func(t *testing.T) {
			var readerCalled atomic.Bool
			s := &moq.Server{
				HTTP2Address: "127.0.0.1:19895",
				HTTP3Address: "127.0.0.1:19896",
				QUICAddress:  "127.0.0.1:19897",
				ServerCert:   test.CreateTempFile(t, test.TLSCertPub),
				ServerKey:    test.CreateTempFile(t, test.TLSCertKey),
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				PathManager: &test.PathManager{AddReaderImpl: func(_ defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
					readerCalled.Store(true)
					return nil, fmt.Errorf("reader unavailable")
				}},
				Parent: test.NilLogger,
			}
			require.NoError(t, s.Initialize())
			defer s.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			d := &webtransport.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				QUICConfig: &quic.Config{
					EnableDatagrams:                  true,
					EnableStreamResetPartialDelivery: true,
				},
				ApplicationProtocols: []string{ca.version.String()},
			}
			defer d.Close() //nolint:errcheck
			res, sx, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream", nil)
			require.NoError(t, err)
			defer sx.CloseWithError(0, "") //nolint:errcheck
			defer res.Body.Close()         //nolint:errcheck
			performWTSetup(ctx, t, sx, ca.version)

			bidi, err := sx.OpenStreamSync(ctx)
			require.NoError(t, err)
			defer bidi.Close() //nolint:errcheck
			body := append([]byte{0x01, 0x00, 0x07}, []byte("catalog")...)
			body = append(body, 0x01)
			body = append(body, ca.filter...)
			_, err = bidi.Write(append([]byte{0x03, 0x00, byte(len(body))}, body...))
			require.NoError(t, err)
			msg, err := controlmessage.Read(bidi, ca.version)
			require.NoError(t, err)
			require.IsType(t, &controlmessage.RequestError{}, msg)
			require.Equal(t, "reader unavailable", msg.(*controlmessage.RequestError).Reason)
			require.True(t, readerCalled.Load())
		})
	}
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

	u, err := url.Parse("moqt://127.0.0.1:19896/teststream")
	require.NoError(t, err)

	client := &protomoq.Client{
		URL:       u,
		Transport: conf.MoQTransportWebTransport,
		TLSConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		Log:       test.NilLogger,
	}
	err = client.Initialize(ctx)
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck

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

	err = client.WriteSubGroup(ctx, 0, 0, nil, cat)
	require.NoError(t, err)

	err = client.Publish(ctx, "catalog", 0, nil)
	require.Error(t, err)

	require.Eventually(t, func() bool {
		return parent.contains("too many catalog tracks: 51")
	}, time.Second, 50*time.Millisecond)
	require.False(t, addPublisherCalled.Load())
}
