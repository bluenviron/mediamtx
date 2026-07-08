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
	inFormat      format.Format
	streamFormat  *streamFormat
	useRTPPackets bool

	rtpDecoder rtpDecoder
}

func (ssf *subStreamFormat) initialize() error {
	if ssf.useRTPPackets {
		var err error
		ssf.rtpDecoder, err = newRTPDecoder(ssf.inFormat)
		if err != nil {
			return err
		}
	}

	if ssf.streamFormat.rtpEncoder == nil && (!ssf.useRTPPackets ||
		ssf.streamFormat.alwaysAvailable ||
		ssf.streamFormat.forceRemux) {
		var err error
		ssf.streamFormat.rtpEncoder, err = newRTPEncoder(
			ssf.streamFormat.outFormat,
			ssf.streamFormat.rtpMaxPayloadSize,
			nil,
			nil)
		if err != nil {
			return err
		}

		ssf.streamFormat.rtpTimeOffset, err = randUint32()
		if err != nil {
			return err
		}
	}

	return nil
}

func (ssf *subStreamFormat) resetRTPEncoder() {
	enc, err := newRTPEncoder(
		ssf.streamFormat.outFormat,
		ssf.streamFormat.rtpMaxPayloadSize,
		nil,
		nil)
	if err != nil {
		ssf.streamFormat.parent.Log(logger.Warn, "failed to reset RTP encoder on codec change: %v", err)
		return
	}
	offset, err := randUint32()
	if err != nil {
		ssf.streamFormat.parent.Log(logger.Warn, "failed to generate RTP time offset on codec change: %v", err)
		return
	}
	ssf.streamFormat.rtpEncoder = enc
	ssf.streamFormat.rtpTimeOffset = offset
}

func (ssf *subStreamFormat) initialize2(liveSource bool, fallbackSwap bool, firstTimeReceived bool, lastPTS time.Duration, lastSystemTime time.Time) {
	if ssf.streamFormat.alwaysAvailable {
		if liveSource {
			ssf.streamFormat.ptsOffset = 0
		} else if firstTimeReceived {
			ptsOffsetGo := lastPTS + time.Since(lastSystemTime)
			ssf.streamFormat.ptsOffset = multiplyAndDivide(int64(ptsOffsetGo),
				int64(ssf.streamFormat.outFormat.ClockRate()), int64(time.Second))
		}
	}

	// on any source swap (alwaysAvailable or fallbackSource, either direction):
	// inject updated video parameter sets and reset the video track's RTP encoder
	// (new SSRC + timestamp offset) so downstream decoders get a clean reset signal.
	// reset is scoped to the video ssf only; the same goroutine that triggers
	// activation owns the video write path, so there is no concurrent reader race.
	// audio SSRC is left unchanged: audio decoders do not need a reset on source swap.
	if ssf.streamFormat.alwaysAvailable || fallbackSwap {
		switch inFormat := ssf.inFormat.(type) {
		case *format.H265:
			if inFormat.VPS != nil && inFormat.SPS != nil && inFormat.PPS != nil {
				if ssf.streamFormat.sourceSwapSSRCReset && ssf.streamFormat.rtpEncoder != nil {
					ssf.streamFormat.parent.Log(logger.Info, "source swap detected, resetting RTP SSRC")
					ssf.resetRTPEncoder()
				}
				ssf.writeUnit(&unit.Unit{
					PTS:        0,
					NTP:        time.Time{},
					RTPPackets: nil,
					Payload:    unit.PayloadH265([][]byte{inFormat.VPS, inFormat.SPS, inFormat.PPS}),
				})
			}

		case *format.H264:
			if inFormat.SPS != nil && inFormat.PPS != nil {
				if ssf.streamFormat.sourceSwapSSRCReset && ssf.streamFormat.rtpEncoder != nil {
					ssf.streamFormat.parent.Log(logger.Info, "source swap detected, resetting RTP SSRC")
					ssf.resetRTPEncoder()
				}
				ssf.writeUnit(&unit.Unit{
					PTS:        0,
					NTP:        time.Time{},
					RTPPackets: nil,
					Payload:    unit.PayloadH264([][]byte{inFormat.SPS, inFormat.PPS}),
				})
			}
		}
	}
}

func (ssf *subStreamFormat) writeUnit(u *unit.Unit) {
	err := ssf.writeUnitInner(u)
	if err != nil {
		ssf.streamFormat.inboundFramesInError.Add(err)
		return
	}
}

func (ssf *subStreamFormat) writeUnitInner(u *unit.Unit) error {
	if ssf.streamFormat.alwaysAvailable {
		u.PTS += ssf.streamFormat.ptsOffset

		ssf.streamFormat.updateLastTime(
			multiplyAndDivide2(time.Duration(u.PTS),
				time.Second, time.Duration(ssf.streamFormat.outFormat.ClockRate())))
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
					ssf.streamFormat.rtpEncoder, err = newRTPEncoder(ssf.streamFormat.outFormat, ssf.streamFormat.rtpMaxPayloadSize,
						new(pkt.SSRC), new(pkt.SequenceNumber))
					if err != nil {
						if _, ok := errors.AsType[rtpEncoderNotAvailableError](err); ok {
							return fmt.Errorf("RTP payload size (%d) is greater than maximum allowed (%d)",
								len(pkt.Payload), ssf.streamFormat.rtpMaxPayloadSize)
						}
						return err
					}

					ssf.streamFormat.rtpTimeOffset = pkt.Timestamp - uint32(u.PTS)

					ssf.streamFormat.parent.Log(logger.Info,
						"RTP packets are too big (%d > %d), remuxing them into smaller ones",
						len(pkt.Payload), ssf.streamFormat.rtpMaxPayloadSize)
					break
				}
			}
		}

		if ssf.streamFormat.rtpEncoder != nil {
			u.RTPPackets = nil
		}
	}

	if !u.NilPayload() {
		ssf.streamFormat.formatUpdater(ssf.streamFormat.outFormat, u.Payload, ssf.streamFormat.updateOutDesc)

		u.Payload = ssf.streamFormat.unitRemuxer(ssf.streamFormat.outFormat, u.Payload)

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
	ssf.streamFormat.inboundBytes.Add(size)

	ssf.streamFormat.writeRTSP(u.RTPPackets, u.NTP)

	for sr, onData := range ssf.streamFormat.onDatas {
		csr := sr
		cOnData := onData
		sr.push(func() error {
			if !csr.SkipOutboundBytes {
				ssf.streamFormat.outboundBytes.Add(size)
			}
			return cOnData(u)
		})
	}

	return nil
}
