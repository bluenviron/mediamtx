package stream

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type streamFormat struct {
	source         logger.Writer
	proc           formatprocessor.Processor
	mutex          sync.RWMutex
	nonRTSPReaders map[interface{}]func(formatprocessor.Unit)
}

func newStreamFormat(
	udpMaxPayloadSize int,
	forma formats.Format,
	generateRTPPackets bool,
	source logger.Writer,
) (*streamFormat, error) {
	proc, err := formatprocessor.New(udpMaxPayloadSize, forma, generateRTPPackets, source)
	if err != nil {
		return nil, err
	}

	sf := &streamFormat{
		source:         source,
		proc:           proc,
		nonRTSPReaders: make(map[interface{}]func(formatprocessor.Unit)),
	}

	return sf, nil
}

func (sf *streamFormat) addReader(r interface{}, cb func(formatprocessor.Unit)) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	sf.nonRTSPReaders[r] = cb
}

func (sf *streamFormat) removeReader(r interface{}) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	delete(sf.nonRTSPReaders, r)
}

func (sf *streamFormat) writeUnit(s *Stream, medi *media.Media, data formatprocessor.Unit) {
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

func (sf *streamFormat) writeRTPPacket(s *Stream, medi *media.Media, pkt *rtp.Packet, ntp time.Time) {
	sf.writeUnit(s, medi, sf.proc.UnitForRTPPacket(pkt, ntp))
}
