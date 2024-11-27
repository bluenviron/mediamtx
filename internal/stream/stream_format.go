package stream

import (
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func unitSize(u unit.Unit) uint64 {
	n := uint64(0)
	for _, pkt := range u.GetRTPPackets() {
		n += uint64(pkt.MarshalSize())
	}
	return n
}

type streamFormat struct {
	udpMaxPayloadSize  int
	format             format.Format
	generateRTPPackets bool
	decodeErrLogger    logger.Writer

	proc           formatprocessor.Processor
	pausedReaders  map[*streamReader]ReadFunc
	runningReaders map[*streamReader]ReadFunc
}

func (sf *streamFormat) initialize() error {
	sf.pausedReaders = make(map[*streamReader]ReadFunc)
	sf.runningReaders = make(map[*streamReader]ReadFunc)

	var err error
	sf.proc, err = formatprocessor.New(sf.udpMaxPayloadSize, sf.format, sf.generateRTPPackets)
	if err != nil {
		return err
	}

	return nil
}

func (sf *streamFormat) addReader(sr *streamReader, cb ReadFunc) {
	sf.pausedReaders[sr] = cb
}

func (sf *streamFormat) removeReader(sr *streamReader) {
	delete(sf.pausedReaders, sr)
	delete(sf.runningReaders, sr)
}

func (sf *streamFormat) startReader(sr *streamReader) {
	if cb, ok := sf.pausedReaders[sr]; ok {
		delete(sf.pausedReaders, sr)
		sf.runningReaders[sr] = cb
	}
}

func (sf *streamFormat) writeUnit(s *Stream, medi *description.Media, u unit.Unit) {
	err := sf.proc.ProcessUnit(u)
	if err != nil {
		sf.decodeErrLogger.Log(logger.Warn, err.Error())
		return
	}

	sf.writeUnitInner(s, medi, u)
}

func (sf *streamFormat) writeRTPPacket(
	s *Stream,
	medi *description.Media,
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
) {
	hasNonRTSPReaders := len(sf.pausedReaders) > 0 || len(sf.runningReaders) > 0

	u, err := sf.proc.ProcessRTPPacket(pkt, ntp, pts, hasNonRTSPReaders)
	if err != nil {
		sf.decodeErrLogger.Log(logger.Warn, err.Error())
		return
	}

	sf.writeUnitInner(s, medi, u)
}

func (sf *streamFormat) writeUnitInner(s *Stream, medi *description.Media, u unit.Unit) {
	size := unitSize(u)

	atomic.AddUint64(s.bytesReceived, size)

	if s.rtspStream != nil {
		for _, pkt := range u.GetRTPPackets() {
			s.rtspStream.WritePacketRTPWithNTP(medi, pkt, u.GetNTP()) //nolint:errcheck
		}
	}

	if s.rtspsStream != nil {
		for _, pkt := range u.GetRTPPackets() {
			s.rtspsStream.WritePacketRTPWithNTP(medi, pkt, u.GetNTP()) //nolint:errcheck
		}
	}

	for sr, cb := range sf.runningReaders {
		ccb := cb
		sr.push(func() error {
			atomic.AddUint64(s.bytesSent, size)
			return ccb(u)
		})
	}
}
