package core

import (
	"sync"
	"sync/atomic"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
)

type streamFormat struct {
	proc           formatProcessor
	mutex          sync.RWMutex
	nonRTSPReaders map[reader]func(data)
}

func newStreamFormat(forma format.Format, generateRTPPackets bool) (*streamFormat, error) {
	proc, err := newFormatProcessor(forma, generateRTPPackets)
	if err != nil {
		return nil, err
	}

	sf := &streamFormat{
		proc:           proc,
		nonRTSPReaders: make(map[reader]func(data)),
	}

	return sf, nil
}

func (sf *streamFormat) readerAdd(r reader, cb func(data)) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	sf.nonRTSPReaders[r] = cb
}

func (sf *streamFormat) readerRemove(r reader) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	delete(sf.nonRTSPReaders, r)
}

func (sf *streamFormat) writeData(s *stream, medi *media.Media, data data) error {
	sf.mutex.RLock()
	defer sf.mutex.RUnlock()

	hasNonRTSPReaders := len(sf.nonRTSPReaders) > 0

	err := sf.proc.process(data, hasNonRTSPReaders)
	if err != nil {
		return err
	}

	// forward RTP packets to RTSP readers
	for _, pkt := range data.getRTPPackets() {
		atomic.AddUint64(s.bytesReceived, uint64(pkt.MarshalSize()))
		s.rtspStream.WritePacketRTPWithNTP(medi, pkt, data.getNTP())
	}

	// forward data to non-RTSP readers
	for _, cb := range sf.nonRTSPReaders {
		cb(data)
	}

	return nil
}
