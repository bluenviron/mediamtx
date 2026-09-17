package webrtc

import (
	"encoding/binary"
	"sync"

	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	timedKLVDataChannelLabel = "KLV-TIMED"
	timedKLVEnvelopeVersion  = 1
	timedKLVHeaderSize       = 8
)

type videoTimestampMapper struct {
	mutex        sync.RWMutex
	valid        bool
	pts          int64
	rtpTimestamp uint32
}

func (m *videoTimestampMapper) update(pts int64, rtpTimestamp uint32) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.valid = true
	m.pts = pts
	m.rtpTimestamp = rtpTimestamp
}

func (m *videoTimestampMapper) translate(pts int64) (uint32, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.valid {
		return 0, false
	}

	// Video and KLV presentation timestamps use the 90 kHz media clock.
	// Conversion to uint32 applies the RTP timestamp wraparound.
	return m.rtpTimestamp + uint32(pts-m.pts), true
}

func marshalTimedKLV(klv unit.PayloadKLV, rtpTimestamp uint32) []byte {
	buf := make([]byte, timedKLVHeaderSize+len(klv))
	buf[0] = timedKLVEnvelopeVersion
	binary.BigEndian.PutUint16(buf[2:4], timedKLVHeaderSize)
	binary.BigEndian.PutUint32(buf[4:8], rtpTimestamp)
	copy(buf[timedKLVHeaderSize:], klv)
	return buf
}
