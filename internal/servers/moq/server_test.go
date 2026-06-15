package moq

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	mch264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/controlmessage"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/require"
)

type serverDummyPath struct{}

func (p *serverDummyPath) Name() string                                  { return "teststream" }
func (p *serverDummyPath) SafeConf() *conf.Path                          { return &conf.Path{} }
func (p *serverDummyPath) ExternalCmdEnv() externalcmd.Environment       { return externalcmd.Environment{} }
func (p *serverDummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {}
func (p *serverDummyPath) RemoveReader(_ defs.PathRemoveReaderReq)       {}

func TestServer(t *testing.T) {
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

	s := &Server{
		HTTP2Address:   "127.0.0.1:19895",
		HTTP3Address:   "127.0.0.1:19896",
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

	d := &webtransport.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		},
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
		ApplicationProtocols: []string{"moqt-18"},
	}
	defer d.Close() //nolint:errcheck

	res, sx, err := d.Dial(ctx, "https://127.0.0.1:19896/teststream/moq", nil)
	require.NoError(t, err)
	defer sx.CloseWithError(0, "") //nolint:errcheck
	defer res.Body.Close()         //nolint:errcheck

	setupStream, err := sx.AcceptUniStream(ctx)
	require.NoError(t, err)
	setupMsg, err := controlmessage.Read(setupStream)
	require.NoError(t, err)
	_, ok := setupMsg.(*controlmessage.Setup)
	require.True(t, ok)

	clientSetup, err := sx.OpenUniStreamSync(ctx)
	require.NoError(t, err)
	_, err = clientSetup.Write(controlmessage.Setup{}.Marshal())
	require.NoError(t, err)

	catalogBidi, err := sx.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = catalogBidi.Write(controlmessage.Subscribe{
		RequestID: 1,
		TrackName: ".catalog",
	}.Marshal())
	require.NoError(t, err)

	catalogOkMsg, err := controlmessage.Read(catalogBidi)
	require.NoError(t, err)
	catalogOk, ok := catalogOkMsg.(*controlmessage.SubscribeOk)
	require.True(t, ok)
	require.Equal(t, uint64(1), catalogOk.TrackAlias)

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
	trackOk, ok := trackOkMsg.(*controlmessage.SubscribeOk)
	require.True(t, ok)
	require.Equal(t, uint64(2), trackOk.TrackAlias)

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

	expectedPayload, err2 := mch264.AVCC([][]byte{test.FormatH264.SPS, test.FormatH264.PPS, {5, 1}}).Marshal()
	require.NoError(t, err2)
	require.Equal(t, expectedPayload, frameSG.Objects[0].Payload)
}
