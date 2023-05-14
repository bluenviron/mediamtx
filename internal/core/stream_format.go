package core

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/rtp"

	"github.com/aler9/mediamtx/internal/formatprocessor"
	"github.com/aler9/mediamtx/internal/logger"
)

type streamFormat struct {
	source         source
	proc           formatprocessor.Processor
	mutex          sync.RWMutex
	nonRTSPReaders map[reader]func(formatprocessor.Unit)
}

func newStreamFormat(
	udpMaxPayloadSize int,
	forma formats.Format,
	generateRTPPackets bool,
	source source,
) (*streamFormat, error) {
	proc, err := formatprocessor.New(udpMaxPayloadSize, forma, generateRTPPackets, source)
	if err != nil {
		return nil, err
	}

	sf := &streamFormat{
		source:         source,
		proc:           proc,
		nonRTSPReaders: make(map[reader]func(formatprocessor.Unit)),
	}

	return sf, nil
}

func (sf *streamFormat) readerAdd(r reader, cb func(formatprocessor.Unit)) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	sf.nonRTSPReaders[r] = cb
}

func (sf *streamFormat) readerRemove(r reader) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	delete(sf.nonRTSPReaders, r)
}

func (sf *streamFormat) writeUnit(s *stream, medi *media.Media, data formatprocessor.Unit) {
	sf.mutex.RLock()
	defer sf.mutex.RUnlock()

	hasNonRTSPReaders := len(sf.nonRTSPReaders) > 0

	err := sf.proc.Process(data, hasNonRTSPReaders)
	if err != nil {
		sf.source.Log(logger.Warn, err.Error())
		return
	}

	// forward RTP packets to RTSP readers
	for _, pkt := range data.GetRTPPackets() {
		atomic.AddUint64(s.bytesReceived, uint64(pkt.MarshalSize()))
		s.rtspStream.WritePacketRTPWithNTP(medi, pkt, data.GetNTP())
	}

	// forward decoded frames to non-RTSP readers
	for _, cb := range sf.nonRTSPReaders {
		cb(data)
	}
}

func (sf *streamFormat) writeRTPPacket(s *stream, medi *media.Media, pkt *rtp.Packet, ntp time.Time) {
	sf.writeUnit(s, medi, sf.proc.UnitForRTPPacket(pkt, ntp))
}
