package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpav1"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorAV1 struct {
	udpMaxPayloadSize int
	format            *format.AV1
	log               logger.Writer

	encoder *rtpav1.Encoder
	decoder *rtpav1.Decoder
}

func newAV1(
	udpMaxPayloadSize int,
	forma *format.AV1,
	generateRTPPackets bool,
	log logger.Writer,
) (*formatProcessorAV1, error) {
	t := &formatProcessorAV1{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
		log:               log,
	}

	if generateRTPPackets {
		err := t.createEncoder()
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *formatProcessorAV1) createEncoder() error {
	t.encoder = &rtpav1.Encoder{
		PayloadMaxSize: t.udpMaxPayloadSize - 12,
	}
	return t.encoder.Init()
}

func (t *formatProcessorAV1) Process(u unit.Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := u.(*unit.AV1)

	if tunit.RTPPackets != nil {
		pkt := tunit.RTPPackets[0]

		// remove padding
		pkt.Header.Padding = false
		pkt.PaddingSize = 0

		if pkt.MarshalSize() > t.udpMaxPayloadSize {
			return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
				pkt.MarshalSize(), t.udpMaxPayloadSize)
		}

		// decode from RTP
		if hasNonRTSPReaders || t.decoder != nil {
			if t.decoder == nil {
				var err error
				t.decoder, err = t.format.CreateDecoder()
				if err != nil {
					return err
				}
			}

			tu, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpav1.ErrNonStartingPacketAndNoPrevious || err == rtpav1.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.TU = tu
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.TU)
	if err != nil {
		return err
	}
	setTimestamp(pkts, tunit.RTPPackets, t.format.ClockRate(), tunit.PTS)
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorAV1) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time, pts time.Duration) Unit {
	return &unit.AV1{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}
}
