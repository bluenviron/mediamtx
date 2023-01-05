package core

import (
	"sync"
	"sync/atomic"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"

	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
)

type streamFormat struct {
	proc           formatprocessor.Processor
	mutex          sync.RWMutex
	nonRTSPReaders map[reader]func(formatprocessor.Data)
}

func newStreamFormat(forma format.Format, generateRTPPackets bool) (*streamFormat, error) {
	proc, err := formatprocessor.New(forma, generateRTPPackets)
	if err != nil {
		return nil, err
	}

	sf := &streamFormat{
		proc:           proc,
		nonRTSPReaders: make(map[reader]func(formatprocessor.Data)),
	}

	return sf, nil
}

func (sf *streamFormat) readerAdd(r reader, cb func(formatprocessor.Data)) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	sf.nonRTSPReaders[r] = cb
}

func (sf *streamFormat) readerRemove(r reader) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	delete(sf.nonRTSPReaders, r)
}

func (sf *streamFormat) writeData(s *stream, medi *media.Media, data formatprocessor.Data) error {
	sf.mutex.RLock()
	defer sf.mutex.RUnlock()

	hasNonRTSPReaders := len(sf.nonRTSPReaders) > 0

	err := sf.proc.Process(data, hasNonRTSPReaders)
	if err != nil {
		return err
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

	return nil
}
