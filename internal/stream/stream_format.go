package stream

import (
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	mch264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	mch265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	mchav1 "github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	mchvp9 "github.com/bluenviron/mediacommon/v2/pkg/codecs/vp9"

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
	processingErrors   *counterdumper.CounterDumper
	parent             logger.Writer

	proc         codecprocessor.Processor
	ntpEstimator *ntpestimator.Estimator
	onDatas      map[*Reader]OnDataFunc
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

	// For video tracks, always decode for key frame detection
	// even if there are no non-RTSP readers
	processForDecode := hasNonRTSPReaders
	if medi.Type == description.MediaTypeVideo && !hasNonRTSPReaders {
		processForDecode = true
	}

	err := sf.proc.ProcessRTPPacket(u, processForDecode)
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

	size := unitSize(u)

	atomic.AddUint64(s.bytesReceived, size)

	// Update last RTP packet timestamp if we have RTP packets
	if len(u.RTPPackets) > 0 && !u.NTP.IsZero() {
		atomic.StoreInt64(s.lastRTPTimestamp, u.NTP.UnixNano())
	}

	// Detect and track key frames for video tracks
	if !u.NTP.IsZero() && medi.Type == description.MediaTypeVideo {
		var codec string
		isKeyFrame := false
		
		// First, try to detect from decoded payload
		if !u.NilPayload() {
			switch payload := u.Payload.(type) {
			case unit.PayloadH264:
				isKeyFrame = mch264.IsRandomAccess(payload)
				codec = sf.format.Codec()
			case unit.PayloadH265:
				isKeyFrame = mch265.IsRandomAccess(payload)
				codec = sf.format.Codec()
			case unit.PayloadVP9:
				var h mchvp9.Header
				if err := h.Unmarshal(payload); err == nil {
					isKeyFrame = !h.NonKeyFrame
					codec = sf.format.Codec()
				}
			case unit.PayloadAV1:
				// Check for Sequence Header OBU which indicates a key frame
				// (Sequence Headers are sent with key frames for random access)
				for _, obu := range payload {
					if len(obu) == 0 {
						continue
					}
					typ := mchav1.OBUType((obu[0] >> 3) & 0b1111)
					
					if typ == mchav1.OBUTypeSequenceHeader {
						isKeyFrame = true
						codec = sf.format.Codec()
						break
					}
				}
			case unit.PayloadVP8:
				// VP8 key frame: first byte bit 0 is 0 for key frame, 1 for delta frame
				if len(payload) > 0 && (payload[0]&0x01) == 0 {
					isKeyFrame = true
					codec = sf.format.Codec()
				}
			}
			
			// Track frame timestamp for FPS calculation (for all video frames, not just key frames)
			if codec == "" {
				codec = sf.format.Codec()
			}
		}
		
		// If payload is nil but we have RTP packets, try to detect from RTP payload
		if codec == "" && len(u.RTPPackets) > 0 {
			switch sf.format.(type) {
			case *format.H264:
				codec = sf.format.Codec()
				// Check for IDR NALU
				for _, pkt := range u.RTPPackets {
					if len(pkt.Payload) == 0 {
						continue
					}
					naluType := pkt.Payload[0] & 0x1F
					if naluType == 5 { // IDR NALU
						isKeyFrame = true
						break
					}
					if naluType == 24 { // STAP-A
						offset := 1
						for offset < len(pkt.Payload) {
							if offset+2 > len(pkt.Payload) {
								break
							}
							naluSize := int(pkt.Payload[offset])<<8 | int(pkt.Payload[offset+1])
							offset += 2
							if offset+naluSize > len(pkt.Payload) {
								break
							}
							if naluSize > 0 {
								naluType := pkt.Payload[offset] & 0x1F
								if naluType == 5 {
									isKeyFrame = true
									break
								}
								offset += naluSize
							}
						}
						if isKeyFrame {
							break
						}
					}
				}
			case *format.H265:
				codec = sf.format.Codec()
				// Check for IDR NALUs
				for _, pkt := range u.RTPPackets {
					if len(pkt.Payload) == 0 {
						continue
					}
					naluType := (pkt.Payload[0] >> 1) & 0x3F
					if naluType == 19 || naluType == 20 || naluType == 21 {
						isKeyFrame = true
						break
					}
					if naluType == 48 { // AP
						offset := 2
						for offset < len(pkt.Payload) {
							if offset+2 > len(pkt.Payload) {
								break
							}
							naluSize := int(pkt.Payload[offset])<<8 | int(pkt.Payload[offset+1])
							offset += 2
							if offset+naluSize > len(pkt.Payload) {
								break
							}
							if naluSize > 0 {
								naluType := (pkt.Payload[offset] >> 1) & 0x3F
								if naluType == 19 || naluType == 20 || naluType == 21 {
									isKeyFrame = true
									break
								}
								offset += naluSize
							}
						}
						if isKeyFrame {
							break
						}
					}
				}
			case *format.VP8:
				codec = sf.format.Codec()
				if len(u.RTPPackets[0].Payload) > 0 {
					if (u.RTPPackets[0].Payload[0] & 0x01) == 0 {
						isKeyFrame = true
					}
				}
			case *format.VP9, *format.AV1:
				codec = sf.format.Codec()
			}
		}

		// Track frame for FPS calculation
		// Only count complete frames (when we have decoded payload)
		// A decoded payload means we have a complete frame, not just an RTP packet
		if codec != "" && !u.NilPayload() {
			currentTS := u.NTP.UnixNano()
			s.fpsMutex.Lock()
			if _, ok := s.frameTimestamps[codec]; !ok {
				s.frameTimestamps[codec] = make([]int64, 0, 60) // pre-allocate for ~1 second at 60fps
			}
			timestamps := s.frameTimestamps[codec]
			// Add current timestamp
			timestamps = append(timestamps, currentTS)
			// Remove timestamps older than 1 second (keep sliding window)
			oneSecondAgo := currentTS - int64(time.Second)
			validStart := 0
			for i, ts := range timestamps {
				if ts >= oneSecondAgo {
					validStart = i
					break
				}
			}
			if validStart > 0 {
				timestamps = timestamps[validStart:]
			}
			s.frameTimestamps[codec] = timestamps
			s.fpsMutex.Unlock()
		}

		// Track key frame for GOP size calculation
		if isKeyFrame && codec != "" {
			s.keyFramesMutex.Lock()
			if _, ok := s.keyFramesCount[codec]; !ok {
				s.keyFramesCount[codec] = new(uint64)
				s.lastKeyFrameTS[codec] = new(int64)
				s.prevKeyFrameTS[codec] = new(int64)
				s.lastGOPSize[codec] = new(int64)
			}
			count := s.keyFramesCount[codec]
			ts := s.lastKeyFrameTS[codec]
			prevTS := s.prevKeyFrameTS[codec]
			gopSize := s.lastGOPSize[codec]
			
			currentTS := u.NTP.UnixNano()
			prevTSVal := atomic.LoadInt64(prevTS)
			
			// Calculate GOP size if we have a previous key frame
			if prevTSVal > 0 {
				gopSizeVal := currentTS - prevTSVal
				if gopSizeVal > 0 {
					atomic.StoreInt64(gopSize, gopSizeVal)
				}
			}
			
			// Update timestamps: previous becomes current, current becomes new
			atomic.StoreInt64(prevTS, atomic.LoadInt64(ts))
			atomic.StoreInt64(ts, currentTS)
			s.keyFramesMutex.Unlock()
			
			atomic.AddUint64(count, 1)
		}
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
