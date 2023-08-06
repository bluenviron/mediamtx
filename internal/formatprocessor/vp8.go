package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpvp8"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitVP8 is a VP8 data unit.
type UnitVP8 struct {
	BaseUnit
	PTS   time.Duration
	Frame []byte
}

type formatProcessorVP8 struct {
	udpMaxPayloadSize int
	format            *formats.VP8
	encoder           *rtpvp8.Encoder
	decoder           *rtpvp8.Decoder
}

func newVP8(
	udpMaxPayloadSize int,
	forma *formats.VP8,
	generateRTPPackets bool,
	_ logger.Writer,
) (*formatProcessorVP8, error) {
	t := &formatProcessorVP8{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if generateRTPPackets {
		err := t.createEncoder()
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *formatProcessorVP8) createEncoder() error {
	t.encoder = &rtpvp8.Encoder{
		PayloadMaxSize: t.udpMaxPayloadSize - 12,
		PayloadType:    t.format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *formatProcessorVP8) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitVP8)

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

			frame, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpvp8.ErrNonStartingPacketAndNoPrevious || err == rtpvp8.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.Frame = frame
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.Frame, tunit.PTS)
	if err != nil {
		return err
	}
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorVP8) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitVP8{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
		},
	}
}
