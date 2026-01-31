package stream

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type subStreamFormat struct {
	curFormat     format.Format
	streamFormat  *streamFormat
	useRTPPackets bool

	rtpDecoder        rtpDecoder
	tempRTPEncoder    rtpEncoder
	tempRTPTimeOffset uint32
}

func (ssf *subStreamFormat) initialize() error {
	if ssf.useRTPPackets {
		var err error
		ssf.rtpDecoder, err = newRTPDecoder(ssf.curFormat)
		if err != nil {
			return err
		}
	}

	if ssf.streamFormat.rtpEncoder == nil && (!ssf.useRTPPackets || ssf.streamFormat.alwaysAvailable) {
		var err error
		ssf.tempRTPEncoder, err = newRTPEncoder(ssf.curFormat, ssf.streamFormat.rtpMaxPayloadSize, nil, nil)
		if err != nil {
			return err
		}

		ssf.tempRTPTimeOffset, err = randUint32()
		if err != nil {
			return err
		}
	}

	return nil
}

func (ssf *subStreamFormat) initialize2() {
	if ssf.tempRTPEncoder != nil {
		if ssf.streamFormat.rtpEncoder == nil {
			ssf.streamFormat.rtpEncoder = ssf.tempRTPEncoder
			ssf.streamFormat.rtpTimeOffset = ssf.tempRTPTimeOffset
		}

		ssf.tempRTPEncoder = nil
		ssf.tempRTPTimeOffset = 0
	}

	if ssf.streamFormat.alwaysAvailable {
		if ssf.streamFormat.firstReceived {
			deltaT := max(1, multiplyAndDivide(
				int64(time.Since(ssf.streamFormat.lastSystemTime)), int64(ssf.streamFormat.format.ClockRate()), int64(time.Second)))
			ssf.streamFormat.ptsOffset = ssf.streamFormat.lastPTS + deltaT
		}

		switch curFormat := ssf.curFormat.(type) {
		case *format.H265:
			sps, pps, vps := curFormat.SafeParams()

			if sps != nil && pps != nil && vps != nil {
				ssf.writeUnit(&unit.Unit{
					PTS:        0,
					NTP:        time.Time{},
					RTPPackets: nil,
					Payload:    unit.PayloadH265([][]byte{sps, pps, vps}),
				})
			}

		case *format.H264:
			sps, pps := curFormat.SafeParams()

			if sps != nil && pps != nil {
				ssf.writeUnit(&unit.Unit{
					PTS:        0,
					NTP:        time.Time{},
					RTPPackets: nil,
					Payload:    unit.PayloadH264([][]byte{sps, pps}),
				})
			}
		}
	}
}

func (ssf *subStreamFormat) writeUnit(u *unit.Unit) {
	err := ssf.writeUnitInner(u)
	if err != nil {
		ssf.streamFormat.processingErrors.Add(err)
		return
	}
}

func (ssf *subStreamFormat) writeUnitInner(u *unit.Unit) error {
	if ssf.streamFormat.alwaysAvailable {
		ssf.streamFormat.firstReceived = true
		u.PTS += ssf.streamFormat.ptsOffset
		if u.PTS > ssf.streamFormat.lastPTS {
			ssf.streamFormat.lastPTS = u.PTS
		}
		ssf.streamFormat.lastSystemTime = time.Now()
	}

	if ssf.streamFormat.replaceNTP {
		u.NTP = ssf.streamFormat.ntpEstimator.Estimate(u.PTS)
	}

	if len(u.RTPPackets) != 0 {
		if ssf.rtpDecoder != nil {
			var err error
			u.Payload, err = ssf.rtpDecoder.decode(u.RTPPackets[0])
			if err != nil {
				return err
			}
		}

		if ssf.streamFormat.rtpEncoder == nil {
			for _, pkt := range u.RTPPackets {
				if len(pkt.Payload) > ssf.streamFormat.rtpMaxPayloadSize {
					var err error
					ssf.streamFormat.rtpEncoder, err = newRTPEncoder(ssf.streamFormat.format, ssf.streamFormat.rtpMaxPayloadSize,
						ptrOf(pkt.SSRC), ptrOf(pkt.SequenceNumber))
					if err != nil {
						var err2 rtpEncoderNotAvailableError
						if errors.As(err, &err2) {
							return fmt.Errorf("RTP payload size (%d) is greater than maximum allowed (%d)",
								len(pkt.Payload), ssf.streamFormat.rtpMaxPayloadSize)
						}
						return err
					}

					ssf.streamFormat.rtpTimeOffset = pkt.Timestamp - uint32(u.PTS)

					ssf.streamFormat.parent.Log(logger.Info, "RTP packets are too big, remuxing them into smaller ones")
					break
				}
			}
		}

		if ssf.streamFormat.rtpEncoder != nil {
			u.RTPPackets = nil
		}
	}

	if !u.NilPayload() {
		ssf.streamFormat.formatUpdater(ssf.streamFormat.format, u.Payload)

		u.Payload = ssf.streamFormat.unitRemuxer(ssf.streamFormat.format, u.Payload)

		if ssf.streamFormat.rtpEncoder != nil && !u.NilPayload() {
			var err error
			u.RTPPackets, err = ssf.streamFormat.rtpEncoder.encode(u.Payload)
			if err != nil {
				return err
			}

			for _, pkt := range u.RTPPackets {
				pkt.Timestamp += ssf.streamFormat.rtpTimeOffset + uint32(u.PTS)
			}
		}
	}

	size := unitSize(u)
	ssf.streamFormat.onBytesReceived(size)

	ssf.streamFormat.writeRTSP(ssf.streamFormat.media, u.RTPPackets, u.NTP)

	for sr, onData := range ssf.streamFormat.onDatas {
		csr := sr
		cOnData := onData
		sr.push(func() error {
			if !csr.SkipBytesSent {
				ssf.streamFormat.onBytesSent(size)
			}
			return cOnData(u)
		})
	}

	return nil
}
