package stream

import (
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/codecprocessor"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/ntpestimator"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func unitSize(u *unit.Unit) uint64 {
	n := uint64(0)
	for _, pkt := range u.RTPPackets {
		n += uint64(pkt.MarshalSize())
	}
	return n
}

type streamFormat struct {
	rtpMaxPayloadSize  int
	format             format.Format
	generateRTPPackets bool
	fillNTP            bool
	dropNonKeyframes   bool
	processingErrors   *counterdumper.CounterDumper
	parent             logger.Writer

	proc         codecprocessor.Processor
	ntpEstimator *ntpestimator.Estimator
	onDatas      map[*Reader]OnDataFunc

	// For codec-agnostic keyframe detection using RTP-level indicators
	lastRTPTimestamp uint32
}

func (sf *streamFormat) initialize() error {
	sf.onDatas = make(map[*Reader]OnDataFunc)

	var err error
	sf.proc, err = codecprocessor.New(sf.rtpMaxPayloadSize, sf.format, sf.generateRTPPackets, sf.parent)
	if err != nil {
		return err
	}

	sf.ntpEstimator = &ntpestimator.Estimator{
		ClockRate: sf.format.ClockRate(),
	}

	return nil
}

func (sf *streamFormat) writeUnit(s *Stream, medi *description.Media, u *unit.Unit) {
	err := sf.proc.ProcessUnit(u)
	if err != nil {
		sf.processingErrors.Increase()
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
	hasNonRTSPReaders := len(sf.onDatas) > 0

	u := &unit.Unit{
		PTS:        pts,
		NTP:        ntp,
		RTPPackets: []*rtp.Packet{pkt},
	}

	err := sf.proc.ProcessRTPPacket(u, hasNonRTSPReaders)
	if err != nil {
		sf.processingErrors.Increase()
		return
	}

	sf.writeUnitInner(s, medi, u)
}


func (sf *streamFormat) writeUnitInner(s *Stream, medi *description.Media, u *unit.Unit) {
	if sf.fillNTP {
		u.NTP = sf.ntpEstimator.Estimate(u.PTS)
	}

	// Drop non-keyframes if configured
	if sf.dropNonKeyframes && medi.Type == description.MediaTypeVideo {
		// Codec-agnostic keyframe detection using RTP-level indicators:
		// - Marker bit: indicates the last packet of a frame (complete frame)
		// - Timestamp change: indicates a new frame
		// Keyframe heuristic: complete frame (marker bit) on a new timestamp
		if len(u.RTPPackets) > 0 {
			firstPkt := u.RTPPackets[0]
			isNewFrame := firstPkt.Timestamp != sf.lastRTPTimestamp

			// Check if any packet has marker bit set (complete frame)
			hasMarker := false
			for _, pkt := range u.RTPPackets {
				if pkt.Marker {
					hasMarker = true
					break
				}
			}

			// Allow complete frames (with marker bit) on new timestamps
			// Also allow the first frame through (when lastRTPTimestamp is 0)
			if hasMarker {
				// Complete frame - allow through if it's a new frame or first frame
				if isNewFrame || sf.lastRTPTimestamp == 0 {
					// Update last timestamp when we output a new frame
					if isNewFrame {
						sf.lastRTPTimestamp = firstPkt.Timestamp
					}
				} else {
					// Same timestamp as last frame - likely a duplicate or continuation, drop it
					return
				}
			} else {
				// Incomplete frame (no marker bit) - drop it
				return
			}
		}
	}

	size := unitSize(u)

	atomic.AddUint64(s.bytesReceived, size)

	// Update last RTP packet timestamp if we have RTP packets
	if len(u.RTPPackets) > 0 && !u.NTP.IsZero() {
		atomic.StoreInt64(s.lastRTPTimestamp, u.NTP.UnixNano())
	}

	if s.rtspStream != nil {
		for _, pkt := range u.RTPPackets {
			s.rtspStream.WritePacketRTPWithNTP(medi, pkt, u.NTP) //nolint:errcheck
		}
	}

	if s.rtspsStream != nil {
		for _, pkt := range u.RTPPackets {
			s.rtspsStream.WritePacketRTPWithNTP(medi, pkt, u.NTP) //nolint:errcheck
		}
	}

	for sr, onData := range sf.onDatas {
		csr := sr
		cOnData := onData
		sr.push(func() error {
			if !csr.SkipBytesSent {
				atomic.AddUint64(s.bytesSent, size)
			}
			return cOnData(u)
		})
	}
}
