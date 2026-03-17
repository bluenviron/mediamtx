package srt

import (
	"bufio"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
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
	var reader *stream.Reader
	defer func() {
		strm.RemoveReader(reader)
	}()
	dataReceived := make(chan struct{})
	dataReceived2 := make(chan struct{})
	n := 0

	pathManager := &test.PathManager{
		FindPathConfImpl: func(req defs.PathFindPathConfReq) (*defs.PathFindPathConfRes, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
			require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
			return &defs.PathFindPathConfRes{Conf: &conf.Path{}, User: req.AccessRequest.Credentials.User}, nil
		},
		AddPublisherImpl: func(req defs.PathAddPublisherReq) (*defs.PathAddPublisherRes, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.True(t, req.AccessRequest.SkipAuth)

			strm = &stream.Stream{
				Desc:              req.Desc,
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

			reader = &stream.Reader{Parent: test.NilLogger}

			reader.OnData(
				strm.Desc.Medias[0],
				strm.Desc.Medias[0].Formats[0],
				func(u *unit.Unit) error {
					switch n {
					case 0:
						require.Equal(t, unit.PayloadH264{
							test.FormatH264.SPS,
							test.FormatH264.PPS,
							{5, 1},
						}, u.Payload)
						close(dataReceived)

					case 1:
						require.Equal(t, unit.PayloadH264{
							test.FormatH264.SPS,
							test.FormatH264.PPS,
							{5, 2},
						}, u.Payload)
						close(dataReceived2)

					default:
						t.Errorf("should not happen")
					}
					n++
					return nil
				})

			strm.AddReader(reader)

			return &defs.PathAddPublisherRes{Path: &dummyPath{}, SubStream: subStream}, nil
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

	track := &mpegts.Track{
		Codec: &tscodecs.H264{},
	}

	bw := bufio.NewWriter(publisher)
	w := &mpegts.Writer{W: bw, Tracks: []*mpegts.Track{track}}
	err = w.Initialize()
	require.NoError(t, err)

	// the MPEG-TS muxer needs two PES packets in order to write the first one

	err = w.WriteH264(track, 0, 0, [][]byte{
		test.FormatH264.SPS,
		test.FormatH264.PPS,
		{5, 1}, // IDR
	})
	require.NoError(t, err)

	err = w.WriteH264(track, 0, 0, [][]byte{
		{5, 2}, // IDR
	})
	require.NoError(t, err)

	err = bw.Flush()
	require.NoError(t, err)

	<-dataReceived

	list, err := s.APIConnsList()
	require.NoError(t, err)
	require.Equal(t, &defs.APISRTConnList{ //nolint:dupl
		Items: []defs.APISRTConn{
			{
				ID:                            list.Items[0].ID,
				Created:                       list.Items[0].Created,
				RemoteAddr:                    list.Items[0].RemoteAddr,
				State:                         "publish",
				Path:                          "teststream",
				Query:                         "param=value",
				User:                          "myuser",
				PacketsSent:                   list.Items[0].PacketsSent,
				PacketsReceived:               list.Items[0].PacketsReceived,
				PacketsSentUnique:             list.Items[0].PacketsSentUnique,
				PacketsReceivedUnique:         list.Items[0].PacketsReceivedUnique,
				PacketsSendLoss:               list.Items[0].PacketsSendLoss,
				PacketsReceivedLoss:           list.Items[0].PacketsReceivedLoss,
				PacketsRetrans:                list.Items[0].PacketsRetrans,
				PacketsReceivedRetrans:        list.Items[0].PacketsReceivedRetrans,
				PacketsSentACK:                list.Items[0].PacketsSentACK,
				PacketsReceivedACK:            list.Items[0].PacketsReceivedACK,
				PacketsSentNAK:                list.Items[0].PacketsSentNAK,
				PacketsReceivedNAK:            list.Items[0].PacketsReceivedNAK,
				PacketsSentKM:                 list.Items[0].PacketsSentKM,
				PacketsReceivedKM:             list.Items[0].PacketsReceivedKM,
				UsSndDuration:                 list.Items[0].UsSndDuration,
				PacketsReceivedBelated:        list.Items[0].PacketsReceivedBelated,
				PacketsSendDrop:               list.Items[0].PacketsSendDrop,
				PacketsReceivedDrop:           list.Items[0].PacketsReceivedDrop,
				PacketsReceivedUndecrypt:      list.Items[0].PacketsReceivedUndecrypt,
				BytesReceived:                 list.Items[0].BytesReceived,
				BytesSent:                     list.Items[0].BytesSent,
				BytesSentUnique:               list.Items[0].BytesSentUnique,
				BytesReceivedUnique:           list.Items[0].BytesReceivedUnique,
				BytesReceivedLoss:             list.Items[0].BytesReceivedLoss,
				BytesRetrans:                  list.Items[0].BytesRetrans,
				BytesReceivedRetrans:          list.Items[0].BytesReceivedRetrans,
				BytesReceivedBelated:          list.Items[0].BytesReceivedBelated,
				BytesSendDrop:                 list.Items[0].BytesSendDrop,
				BytesReceivedDrop:             list.Items[0].BytesReceivedDrop,
				BytesReceivedUndecrypt:        list.Items[0].BytesReceivedUndecrypt,
				OutboundFramesDiscarded:       list.Items[0].OutboundFramesDiscarded,
				UsPacketsSendPeriod:           list.Items[0].UsPacketsSendPeriod,
				PacketsFlowWindow:             list.Items[0].PacketsFlowWindow,
				PacketsFlightSize:             list.Items[0].PacketsFlightSize,
				MsRTT:                         list.Items[0].MsRTT,
				MbpsSendRate:                  list.Items[0].MbpsSendRate,
				MbpsReceiveRate:               list.Items[0].MbpsReceiveRate,
				MbpsLinkCapacity:              list.Items[0].MbpsLinkCapacity,
				BytesAvailSendBuf:             list.Items[0].BytesAvailSendBuf,
				BytesAvailReceiveBuf:          list.Items[0].BytesAvailReceiveBuf,
				MbpsMaxBW:                     list.Items[0].MbpsMaxBW,
				ByteMSS:                       list.Items[0].ByteMSS,
				PacketsSendBuf:                list.Items[0].PacketsSendBuf,
				BytesSendBuf:                  list.Items[0].BytesSendBuf,
				MsSendBuf:                     list.Items[0].MsSendBuf,
				MsSendTsbPdDelay:              list.Items[0].MsSendTsbPdDelay,
				PacketsReceiveBuf:             list.Items[0].PacketsReceiveBuf,
				BytesReceiveBuf:               list.Items[0].BytesReceiveBuf,
				MsReceiveBuf:                  list.Items[0].MsReceiveBuf,
				MsReceiveTsbPdDelay:           list.Items[0].MsReceiveTsbPdDelay,
				PacketsReorderTolerance:       list.Items[0].PacketsReorderTolerance,
				PacketsReceivedAvgBelatedTime: list.Items[0].PacketsReceivedAvgBelatedTime,
				PacketsSendLossRate:           list.Items[0].PacketsSendLossRate,
				PacketsReceivedLossRate:       list.Items[0].PacketsReceivedLossRate,
			},
		},
	}, list)

	// the second PES is written after writer is closed
	publisher.Close()
	<-dataReceived2
}

func TestServerRead(t *testing.T) {
	externalCmdPool := &externalcmd.Pool{}
	externalCmdPool.Initialize()
	defer externalCmdPool.Close()

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

	pathManager := &test.PathManager{
		AddReaderImpl: func(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.Credentials.User)
			require.Equal(t, "mypass", req.AccessRequest.Credentials.Pass)
			return &defs.PathAddReaderRes{Path: &dummyPath{}, User: req.AccessRequest.Credentials.User, Stream: strm}, nil
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

	strm.WaitForReaders()

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		NTP: time.Time{},
		Payload: unit.PayloadH264{
			{5, 1}, // IDR
		},
	})

	r := &mpegts.Reader{R: reader}
	err = r.Initialize()
	require.NoError(t, err)

	require.Equal(t, []*mpegts.Track{{
		PID:   256,
		Codec: &tscodecs.H264{},
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

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		NTP: time.Time{},
		Payload: unit.PayloadH264{
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

	list, err := s.APIConnsList()
	require.NoError(t, err)
	require.Equal(t, &defs.APISRTConnList{ //nolint:dupl
		Items: []defs.APISRTConn{
			{
				ID:                            list.Items[0].ID,
				Created:                       list.Items[0].Created,
				RemoteAddr:                    list.Items[0].RemoteAddr,
				State:                         "read",
				Path:                          "teststream",
				Query:                         "param=value",
				User:                          "myuser",
				PacketsSent:                   list.Items[0].PacketsSent,
				PacketsReceived:               list.Items[0].PacketsReceived,
				PacketsSentUnique:             list.Items[0].PacketsSentUnique,
				PacketsReceivedUnique:         list.Items[0].PacketsReceivedUnique,
				PacketsSendLoss:               list.Items[0].PacketsSendLoss,
				PacketsReceivedLoss:           list.Items[0].PacketsReceivedLoss,
				PacketsRetrans:                list.Items[0].PacketsRetrans,
				PacketsReceivedRetrans:        list.Items[0].PacketsReceivedRetrans,
				PacketsSentACK:                list.Items[0].PacketsSentACK,
				PacketsReceivedACK:            list.Items[0].PacketsReceivedACK,
				PacketsSentNAK:                list.Items[0].PacketsSentNAK,
				PacketsReceivedNAK:            list.Items[0].PacketsReceivedNAK,
				PacketsSentKM:                 list.Items[0].PacketsSentKM,
				PacketsReceivedKM:             list.Items[0].PacketsReceivedKM,
				UsSndDuration:                 list.Items[0].UsSndDuration,
				PacketsReceivedBelated:        list.Items[0].PacketsReceivedBelated,
				PacketsSendDrop:               list.Items[0].PacketsSendDrop,
				PacketsReceivedDrop:           list.Items[0].PacketsReceivedDrop,
				PacketsReceivedUndecrypt:      list.Items[0].PacketsReceivedUndecrypt,
				BytesReceived:                 list.Items[0].BytesReceived,
				BytesSent:                     list.Items[0].BytesSent,
				BytesSentUnique:               list.Items[0].BytesSentUnique,
				BytesReceivedUnique:           list.Items[0].BytesReceivedUnique,
				BytesReceivedLoss:             list.Items[0].BytesReceivedLoss,
				BytesRetrans:                  list.Items[0].BytesRetrans,
				BytesReceivedRetrans:          list.Items[0].BytesReceivedRetrans,
				BytesReceivedBelated:          list.Items[0].BytesReceivedBelated,
				BytesSendDrop:                 list.Items[0].BytesSendDrop,
				BytesReceivedDrop:             list.Items[0].BytesReceivedDrop,
				BytesReceivedUndecrypt:        list.Items[0].BytesReceivedUndecrypt,
				UsPacketsSendPeriod:           list.Items[0].UsPacketsSendPeriod,
				PacketsFlowWindow:             list.Items[0].PacketsFlowWindow,
				PacketsFlightSize:             list.Items[0].PacketsFlightSize,
				MsRTT:                         list.Items[0].MsRTT,
				MbpsSendRate:                  list.Items[0].MbpsSendRate,
				MbpsReceiveRate:               list.Items[0].MbpsReceiveRate,
				MbpsLinkCapacity:              list.Items[0].MbpsLinkCapacity,
				BytesAvailSendBuf:             list.Items[0].BytesAvailSendBuf,
				BytesAvailReceiveBuf:          list.Items[0].BytesAvailReceiveBuf,
				MbpsMaxBW:                     list.Items[0].MbpsMaxBW,
				ByteMSS:                       list.Items[0].ByteMSS,
				PacketsSendBuf:                list.Items[0].PacketsSendBuf,
				BytesSendBuf:                  list.Items[0].BytesSendBuf,
				MsSendBuf:                     list.Items[0].MsSendBuf,
				MsSendTsbPdDelay:              list.Items[0].MsSendTsbPdDelay,
				PacketsReceiveBuf:             list.Items[0].PacketsReceiveBuf,
				BytesReceiveBuf:               list.Items[0].BytesReceiveBuf,
				MsReceiveBuf:                  list.Items[0].MsReceiveBuf,
				MsReceiveTsbPdDelay:           list.Items[0].MsReceiveTsbPdDelay,
				PacketsReorderTolerance:       list.Items[0].PacketsReorderTolerance,
				PacketsReceivedAvgBelatedTime: list.Items[0].PacketsReceivedAvgBelatedTime,
				PacketsSendLossRate:           list.Items[0].PacketsSendLossRate,
				PacketsReceivedLossRate:       list.Items[0].PacketsReceivedLossRate,
			},
		},
	}, list)
}
