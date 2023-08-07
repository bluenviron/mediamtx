package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpav1"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitAV1 is an AV1 data unit.
type UnitAV1 struct {
	BaseUnit
	PTS time.Duration
	TU  [][]byte
}

type formatProcessorAV1 struct {
	udpMaxPayloadSize int
	format            *formats.AV1
	log               logger.Writer

	encoder *rtpav1.Encoder
	decoder *rtpav1.Decoder
}

func newAV1(
	udpMaxPayloadSize int,
	forma *formats.AV1,
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

func (t *formatProcessorAV1) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitAV1)

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
				t.decoder, err = t.format.CreateDecoder2()
				if err != nil {
					return err
				}
			}

			// DecodeUntilMarker() is necessary, otherwise Encode() generates partial groups
			tu, pts, err := t.decoder.DecodeUntilMarker(pkt)
			if err != nil {
				if err == rtpav1.ErrNonStartingPacketAndNoPrevious || err == rtpav1.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.TU = tu
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.TU, tunit.PTS)
	if err != nil {
		return err
	}
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorAV1) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitAV1{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
		},
	}
}
