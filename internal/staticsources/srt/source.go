// Package srt contains the SRT static source.
package srt

import (
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	srt "github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a SRT static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      parent

	mutex sync.RWMutex
	conn  srt.Conn
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[SRT source] "+format, args...)
}

// Info returns runtime information.
func (s *Source) Info() defs.StaticSourceInfo {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.conn == nil {
		return defs.StaticSourceInfo{}
	}

	var stats srt.Statistics
	s.conn.Stats(&stats)

	return defs.StaticSourceInfo{
		TypeSpecific: &defs.APIStaticSourceTypeSpecificSRT{
			RemoteAddr:                    s.conn.RemoteAddr().String(),
			PacketsSent:                   stats.Accumulated.PktSent,
			PacketsReceived:               stats.Accumulated.PktRecv,
			PacketsSentUnique:             stats.Accumulated.PktSentUnique,
			PacketsReceivedUnique:         stats.Accumulated.PktRecvUnique,
			PacketsSendLoss:               stats.Accumulated.PktSendLoss,
			PacketsReceivedLoss:           stats.Accumulated.PktRecvLoss,
			PacketsRetrans:                stats.Accumulated.PktRetrans,
			PacketsReceivedRetrans:        stats.Accumulated.PktRecvRetrans,
			PacketsSentACK:                stats.Accumulated.PktSentACK,
			PacketsReceivedACK:            stats.Accumulated.PktRecvACK,
			PacketsSentNAK:                stats.Accumulated.PktSentNAK,
			PacketsReceivedNAK:            stats.Accumulated.PktRecvNAK,
			PacketsSentKM:                 stats.Accumulated.PktSentKM,
			PacketsReceivedKM:             stats.Accumulated.PktRecvKM,
			UsSndDuration:                 stats.Accumulated.UsSndDuration,
			PacketsReceivedBelated:        stats.Accumulated.PktRecvBelated,
			PacketsSendDrop:               stats.Accumulated.PktSendDrop,
			PacketsReceivedDrop:           stats.Accumulated.PktRecvDrop,
			PacketsReceivedUndecrypt:      stats.Accumulated.PktRecvUndecrypt,
			BytesSent:                     stats.Accumulated.ByteSent,
			BytesReceived:                 stats.Accumulated.ByteRecv,
			BytesSentUnique:               stats.Accumulated.ByteSentUnique,
			BytesReceivedUnique:           stats.Accumulated.ByteRecvUnique,
			BytesReceivedLoss:             stats.Accumulated.ByteRecvLoss,
			BytesRetrans:                  stats.Accumulated.ByteRetrans,
			BytesReceivedRetrans:          stats.Accumulated.ByteRecvRetrans,
			BytesReceivedBelated:          stats.Accumulated.ByteRecvBelated,
			BytesSendDrop:                 stats.Accumulated.ByteSendDrop,
			BytesReceivedDrop:             stats.Accumulated.ByteRecvDrop,
			BytesReceivedUndecrypt:        stats.Accumulated.ByteRecvUndecrypt,
			UsPacketsSendPeriod:           stats.Instantaneous.UsPktSendPeriod,
			PacketsFlowWindow:             stats.Instantaneous.PktFlowWindow,
			PacketsFlightSize:             stats.Instantaneous.PktFlightSize,
			MsRTT:                         stats.Instantaneous.MsRTT,
			MbpsSendRate:                  stats.Instantaneous.MbpsSentRate,
			MbpsReceiveRate:               stats.Instantaneous.MbpsRecvRate,
			MbpsLinkCapacity:              stats.Instantaneous.MbpsLinkCapacity,
			BytesAvailSendBuf:             stats.Instantaneous.ByteAvailSendBuf,
			BytesAvailReceiveBuf:          stats.Instantaneous.ByteAvailRecvBuf,
			MbpsMaxBW:                     stats.Instantaneous.MbpsMaxBW,
			ByteMSS:                       stats.Instantaneous.ByteMSS,
			PacketsSendBuf:                stats.Instantaneous.PktSendBuf,
			BytesSendBuf:                  stats.Instantaneous.ByteSendBuf,
			MsSendBuf:                     stats.Instantaneous.MsSendBuf,
			MsSendTsbPdDelay:              stats.Instantaneous.MsSendTsbPdDelay,
			PacketsReceiveBuf:             stats.Instantaneous.PktRecvBuf,
			BytesReceiveBuf:               stats.Instantaneous.ByteRecvBuf,
			MsReceiveBuf:                  stats.Instantaneous.MsRecvBuf,
			MsReceiveTsbPdDelay:           stats.Instantaneous.MsRecvTsbPdDelay,
			PacketsReorderTolerance:       stats.Instantaneous.PktReorderTolerance,
			PacketsReceivedAvgBelatedTime: stats.Instantaneous.PktRecvAvgBelatedTime,
			PacketsSendLossRate:           stats.Instantaneous.PktSendLossRate,
			PacketsReceivedLossRate:       stats.Instantaneous.PktRecvLossRate,
		},
	}
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	conf := srt.DefaultConfig()
	address, err := conf.UnmarshalURL(params.ResolvedSource)
	if err != nil {
		return err
	}

	err = conf.Validate()
	if err != nil {
		return err
	}

	sconn, err := srt.DialWithContext(params.Context, "srt", address, conf)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	s.conn = sconn
	s.mutex.Unlock()

	defer func() {
		s.mutex.Lock()
		s.conn = nil
		s.mutex.Unlock()
	}()

	readDone := make(chan error)
	go func() {
		readDone <- s.runReader(sconn)
	}()

	for {
		select {
		case err = <-readDone:
			sconn.Close()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			sconn.Close()
			<-readDone
			return nil
		}
	}
}

func (s *Source) runReader(sconn srt.Conn) error {
	sconn.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
	r := &mpegts.EnhancedReader{R: sconn}
	err := r.Initialize()
	if err != nil {
		return err
	}

	decodeErrors := &errordumper.Dumper{
		OnReport: func(val uint64, last error) {
			if val == 1 {
				s.Log(logger.Warn, "decode error: %v", last)
			} else {
				s.Log(logger.Warn, "%d decode errors, last was: %v", val, last)
			}
		},
	}

	decodeErrors.Start()
	defer decodeErrors.Stop()

	r.OnDecodeError(func(err error) {
		decodeErrors.Add(err)
	})

	var subStream *stream.SubStream

	medias, err := mpegts.ToStream(r, &subStream, s)
	if err != nil {
		return err
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: false,
		ReplaceNTP:    true,
	})
	if res.Err != nil {
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = res.SubStream

	for {
		sconn.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err = r.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeSRTSource,
		ID:   "",
	}
}
