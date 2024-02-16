package stream

import (
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
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
	decodeErrLogger logger.Writer
	proc            formatprocessor.Processor
	readers         map[*asyncwriter.Writer]ReadFunc
}

func newStreamFormat(
	udpMaxPayloadSize int,
	forma format.Format,
	generateRTPPackets bool,
	decodeErrLogger logger.Writer,
) (*streamFormat, error) {
	proc, err := formatprocessor.New(udpMaxPayloadSize, forma, generateRTPPackets)
	if err != nil {
		return nil, err
	}

	sf := &streamFormat{
		decodeErrLogger: decodeErrLogger,
		proc:            proc,
		readers:         make(map[*asyncwriter.Writer]ReadFunc),
	}

	return sf, nil
}

func (sf *streamFormat) addReader(r *asyncwriter.Writer, cb ReadFunc) {
	sf.readers[r] = cb
}

func (sf *streamFormat) removeReader(r *asyncwriter.Writer) {
	delete(sf.readers, r)
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
	pts time.Duration,
) {
	hasNonRTSPReaders := len(sf.readers) > 0

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

	for writer, cb := range sf.readers {
		ccb := cb
		writer.Push(func() error {
			atomic.AddUint64(s.bytesSent, size)
			return ccb(u)
		})
	}
}
