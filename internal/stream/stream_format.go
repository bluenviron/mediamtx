package stream

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/errordumper"
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

func randUint32() (uint32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

type streamFormat struct {
	format            format.Format
	media             *description.Media
	useRTPPackets     bool
	rtpMaxPayloadSize int
	replaceNTP        bool
	processingErrors  *errordumper.Dumper
	onBytesReceived   func(uint64)
	onBytesSent       func(uint64)
	writeRTSP         func(*description.Media, []*rtp.Packet, time.Time)
	parent            logger.Writer

	rtpDecoder    rtpDecoder
	formatUpdater formatUpdater
	unitRemuxer   unitRemuxer
	rtpEncoder    rtpEncoder
	ptsOffset     uint32
	ntpEstimator  *ntpestimator.Estimator
	onDatas       map[*Reader]OnDataFunc
}

func (sf *streamFormat) initialize() error {
	sf.onDatas = make(map[*Reader]OnDataFunc)

	if sf.useRTPPackets {
		var err error
		sf.rtpDecoder, err = newRTPDecoder(sf.format)
		if err != nil {
			return err
		}
	}

	sf.formatUpdater = newFormatUpdater(sf.format)
	sf.unitRemuxer = newUnitRemuxer(sf.format)

	if !sf.useRTPPackets {
		var err error
		sf.rtpEncoder, err = newRTPEncoder(sf.format, sf.rtpMaxPayloadSize, nil, nil)
		if err != nil {
			return err
		}

		if sf.rtpEncoder == nil {
			return fmt.Errorf("RTP encoder not available for format %T", sf.format)
		}

		sf.ptsOffset, err = randUint32()
		if err != nil {
			return err
		}
	}

	if sf.replaceNTP {
		sf.ntpEstimator = &ntpestimator.Estimator{
			ClockRate: sf.format.ClockRate(),
		}
	}

	return nil
}

func (sf *streamFormat) writeUnit(u *unit.Unit) {
	err := sf.writeUnitInner(u)
	if err != nil {
		sf.processingErrors.Add(err)
		return
	}
}

func (sf *streamFormat) writeUnitInner(u *unit.Unit) error {
	if sf.useRTPPackets {
		if len(u.RTPPackets) != 1 {
			panic("should not happen")
		}
		if !u.NilPayload() {
			panic("should not happen")
		}

		if sf.rtpDecoder != nil {
			var err error
			u.Payload, err = sf.rtpDecoder.decode(u.RTPPackets[0])
			if err != nil {
				return err
			}
		}

		if sf.rtpEncoder == nil {
			for _, pkt := range u.RTPPackets {
				if len(pkt.Payload) > sf.rtpMaxPayloadSize {
					var err error
					sf.rtpEncoder, err = newRTPEncoder(sf.format, sf.rtpMaxPayloadSize, ptrOf(pkt.SSRC), ptrOf(pkt.SequenceNumber))
					if err != nil {
						return err
					}

					if sf.rtpEncoder == nil {
						return fmt.Errorf("RTP payload size (%d) is greater than maximum allowed (%d)",
							len(pkt.Payload), sf.rtpMaxPayloadSize)
					}

					sf.ptsOffset = pkt.Timestamp - uint32(u.PTS)

					sf.parent.Log(logger.Info, "RTP packets are too big, remuxing them into smaller ones")
					break
				}
			}
		}

		if sf.rtpEncoder != nil {
			u.RTPPackets = nil
		}
	} else {
		if len(u.RTPPackets) != 0 {
			panic("should not happen")
		}
		if u.NilPayload() {
			panic("should not happen")
		}
	}

	if !u.NilPayload() {
		sf.formatUpdater(sf.format, u.Payload)

		u.Payload = sf.unitRemuxer(sf.format, u.Payload)

		if sf.rtpEncoder != nil && !u.NilPayload() {
			var err error
			u.RTPPackets, err = sf.rtpEncoder.encode(u.Payload)
			if err != nil {
				return err
			}

			for _, pkt := range u.RTPPackets {
				pkt.Timestamp += sf.ptsOffset + uint32(u.PTS)
			}
		}
	}

	if sf.replaceNTP {
		u.NTP = sf.ntpEstimator.Estimate(u.PTS)
	}

	size := unitSize(u)
	sf.onBytesReceived(size)

	sf.writeRTSP(sf.media, u.RTPPackets, u.NTP)

	for sr, onData := range sf.onDatas {
		csr := sr
		cOnData := onData
		sr.push(func() error {
			if !csr.SkipBytesSent {
				sf.onBytesSent(size)
			}
			return cOnData(u)
		})
	}

	return nil
}
