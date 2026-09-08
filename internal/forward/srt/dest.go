// Package srt contains the SRT forward destination.
package srt

import (
	"bufio"
	"context"
	"fmt"
	"sync"
	"time"

	srtlib "github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func maxPayloadSize(v int) int {
	return ((v - 16) / 188) * 188
}

// Dest is a SRT forward destination.
type Dest struct {
	Stream            *stream.Stream
	Dest              string
	WriteTimeout      conf.Duration
	UDPMaxPayloadSize int
	Parent            logger.Writer

	mutex   sync.RWMutex
	address string
	conn    srtlib.Conn
	reader  *stream.Reader
}

// Log implements logger.Writer.
func (d *Dest) Log(level logger.Level, format string, args ...any) {
	d.Parent.Log(level, format, args...)
}

// Info returns runtime information.
func (d *Dest) Info() defs.ForwardDestInfo {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if d.conn == nil {
		return defs.ForwardDestInfo{}
	}

	var stats srtlib.Statistics
	d.conn.Stats(&stats)

	return defs.ForwardDestInfo{
		OutboundBytes: stats.Accumulated.ByteSent,
		TypeSpecific: &defs.APIForwardDestTypeSpecificSRT{
			RemoteAddr:                    d.conn.RemoteAddr().String(),
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
			OutboundFramesDiscarded:       d.reader.OutboundFramesDiscarded(),
		},
	}
}

// Run runs the destination.
func (d *Dest) Run(ctx context.Context) error {
	srtConf := srtlib.DefaultConfig()
	address, err := srtConf.UnmarshalURL(d.Dest)
	if err != nil {
		return err
	}

	udpMaxPayloadSize := d.UDPMaxPayloadSize
	if udpMaxPayloadSize == 0 {
		udpMaxPayloadSize = 1472
	}
	srtConf.PayloadSize = uint32(maxPayloadSize(udpMaxPayloadSize))

	err = srtConf.Validate()
	if err != nil {
		return err
	}

	terminate := make(chan struct{})

	errChan := make(chan error)
	go func() {
		errChan <- d.runInner(ctx, address, srtConf, terminate)
	}()

	select {
	case err = <-errChan:
		return err

	case <-ctx.Done():
		close(terminate)
		<-errChan
		return fmt.Errorf("terminated")
	}
}

func (d *Dest) runInner(ctx context.Context, address string, srtConf srtlib.Config, terminate <-chan struct{}) error {
	conn, err := srtlib.DialWithContext(ctx, "srt", address, srtConf)
	if err != nil {
		return err
	}
	defer conn.Close()

	r := &stream.Reader{Parent: d}

	d.mutex.Lock()
	d.address = address
	d.conn = conn
	d.reader = r
	d.mutex.Unlock()

	defer func() {
		d.mutex.Lock()
		d.address = ""
		d.conn = nil
		d.reader = nil
		d.mutex.Unlock()
	}()

	bw := bufio.NewWriterSize(conn, int(srtConf.PayloadSize))

	err = mpegts.FromStream(d.Stream.OrigDesc, r, bw, conn, time.Duration(d.WriteTimeout))
	if err != nil {
		return err
	}

	d.Stream.AddReader(r)
	defer d.Stream.RemoveReader(r)

	select {
	case readErr := <-r.Error():
		return readErr

	case <-terminate:
		return nil
	}
}
