package webrtc

import (
	"encoding/binary"

	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	rawKLVDataChannelLabel   = "KLV"
	timedKLVDataChannelLabel = "KLV-TIMED"
	timedKLVEnvelopeVersion  = 1
	timedKLVHeaderSize       = 8

	timedKLVVersionOffset      = 0
	timedKLVFlagsOffset        = 1
	timedKLVHeaderLengthOffset = 2
	timedKLVRTPTimeOffset      = 4
	timedKLVPayloadOffset      = timedKLVHeaderSize
	timedKLVFlagsNone          = 0
)

// stream.Reader invokes all media callbacks serially. The latest video sample
// is sufficient while the outgoing RTP-to-PTS offset remains constant, and
// re-anchors the mapping when that offset changes.
type videoTimestampMapper struct {
	valid        bool
	pts          int64
	rtpTimestamp uint32
}

func (m *videoTimestampMapper) update(pts int64, rtpTimestamp uint32) {
	m.valid = true
	m.pts = pts
	m.rtpTimestamp = rtpTimestamp
}

func (m *videoTimestampMapper) translate(pts int64) (uint32, bool) {
	if !m.valid {
		return 0, false
	}

	// MediaMTX gives tracks a common PTS origin. Video and KLV both use a
	// 90 kHz clock, so their PTS difference can be applied to the outgoing
	// video RTP timestamp. The uint32 conversion preserves RTP wraparound.
	return m.rtpTimestamp + uint32(pts-m.pts), true
}

func marshalTimedKLV(klv unit.PayloadKLV, rtpTimestamp uint32) []byte {
	buf := make([]byte, timedKLVHeaderSize+len(klv))
	buf[timedKLVVersionOffset] = timedKLVEnvelopeVersion
	buf[timedKLVFlagsOffset] = timedKLVFlagsNone
	binary.BigEndian.PutUint16(
		buf[timedKLVHeaderLengthOffset:timedKLVRTPTimeOffset],
		timedKLVHeaderSize)
	binary.BigEndian.PutUint32(
		buf[timedKLVRTPTimeOffset:timedKLVPayloadOffset],
		rtpTimestamp)
	copy(buf[timedKLVPayloadOffset:], klv)
	return buf
}
