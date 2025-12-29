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

	// For keyframe-only mode
	lastRTPTimestamp       uint32
	currentFrameIsKeyframe bool
	outputSeqNum           uint16
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

// isKeyframePacket checks if the first packet of a frame indicates a keyframe
func (sf *streamFormat) isKeyframePacket(pkt *rtp.Packet) bool {
	if len(pkt.Payload) == 0 {
		return false
	}

	switch sf.format.(type) {
	case *format.H264:
		nalType := pkt.Payload[0] & 0x1F
		switch nalType {
		case 5: // IDR
			return true
		case 7, 8: // SPS, PPS - usually precede IDR in same frame
			return true
		case 28, 29: // FU-A, FU-B
			if len(pkt.Payload) >= 2 {
				fuNalType := pkt.Payload[1] & 0x1F
				if fuNalType == 5 { // IDR
					return true
				}
			}
		case 24: // STAP-A
			payload := pkt.Payload[1:]
			for len(payload) > 2 {
				size := uint16(payload[0])<<8 | uint16(payload[1])
				payload = payload[2:]
				if int(size) > len(payload) {
					break
				}
				if size > 0 {
					innerNalType := payload[0] & 0x1F
					if innerNalType == 5 || innerNalType == 7 || innerNalType == 8 {
						return true
					}
				}
				payload = payload[size:]
			}
		}
		return false

	case *format.H265:
		nalType := (pkt.Payload[0] >> 1) & 0x3F
		switch nalType {
		case 19, 20, 21: // IDR_W_RADL, IDR_N_LP, CRA_NUT
			return true
		case 32, 33, 34: // VPS, SPS, PPS
			return true
		case 49: // FU
			if len(pkt.Payload) >= 3 {
				fuNalType := pkt.Payload[2] & 0x3F
				if fuNalType == 19 || fuNalType == 20 || fuNalType == 21 {
					return true
				}
			}
		case 48: // AP
			payload := pkt.Payload[2:]
			for len(payload) > 2 {
				size := uint16(payload[0])<<8 | uint16(payload[1])
				payload = payload[2:]
				if int(size) > len(payload) || size < 2 {
					break
				}
				innerNalType := (payload[0] >> 1) & 0x3F
				if innerNalType == 19 || innerNalType == 20 || innerNalType == 21 ||
					innerNalType == 32 || innerNalType == 33 || innerNalType == 34 {
					return true
				}
				payload = payload[size:]
			}
		}
		return false

	default:
		// For other codecs, allow all frames
		return true
	}
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
		if len(u.RTPPackets) > 0 {
			firstPkt := u.RTPPackets[0]
			isNewFrame := firstPkt.Timestamp != sf.lastRTPTimestamp

			// On new frame, check if it's a keyframe by inspecting NAL type
			if isNewFrame {
				sf.lastRTPTimestamp = firstPkt.Timestamp
				sf.currentFrameIsKeyframe = sf.isKeyframePacket(firstPkt)
			}

			// Drop if not part of a keyframe
			if !sf.currentFrameIsKeyframe {
				return
			}
		}
	}

	// Rewrite sequence numbers when dropping non-keyframes to ensure continuity
	if sf.dropNonKeyframes && medi.Type == description.MediaTypeVideo {
		for _, pkt := range u.RTPPackets {
			pkt.SequenceNumber = sf.outputSeqNum
			sf.outputSeqNum++
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
