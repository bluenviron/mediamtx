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

type streamFormat struct {
	decodeErrLogger logger.Writer
	proc            formatprocessor.Processor
	nonRTSPReaders  map[interface{}]func(unit.Unit)
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
		nonRTSPReaders:  make(map[interface{}]func(unit.Unit)),
	}

	return sf, nil
}

func (sf *streamFormat) addReader(r interface{}, cb func(unit.Unit)) {
	sf.nonRTSPReaders[r] = cb
}

func (sf *streamFormat) removeReader(r interface{}) {
	delete(sf.nonRTSPReaders, r)
}

func (sf *streamFormat) writeUnit(s *Stream, medi *description.Media, data unit.Unit) {
	hasNonRTSPReaders := len(sf.nonRTSPReaders) > 0

	err := sf.proc.Process(data, hasNonRTSPReaders)
	if err != nil {
		sf.decodeErrLogger.Log(logger.Warn, err.Error())
		return
	}

	n := uint64(0)
	for _, pkt := range data.GetRTPPackets() {
		n += uint64(pkt.MarshalSize())
	}
	atomic.AddUint64(s.bytesReceived, n)

	if s.rtspStream != nil {
		for _, pkt := range data.GetRTPPackets() {
			s.rtspStream.WritePacketRTPWithNTP(medi, pkt, data.GetNTP()) //nolint:errcheck
		}
	}

	if s.rtspsStream != nil {
		for _, pkt := range data.GetRTPPackets() {
			s.rtspsStream.WritePacketRTPWithNTP(medi, pkt, data.GetNTP()) //nolint:errcheck
		}
	}

	// forward decoded frames to non-RTSP readers
	for _, cb := range sf.nonRTSPReaders {
		cb(data)
	}
}

func (sf *streamFormat) writeRTPPacket(
	s *Stream,
	medi *description.Media,
	pkt *rtp.Packet,
	ntp time.Time,
	pts time.Duration,
) {
	sf.writeUnit(s, medi, sf.proc.UnitForRTPPacket(pkt, ntp, pts))
}
