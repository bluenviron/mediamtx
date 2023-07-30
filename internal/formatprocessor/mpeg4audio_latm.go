package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpmpeg4audiolatm"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitMPEG4AudioLATM is a MPEG-4 Audio data unit.
type UnitMPEG4AudioLATM struct {
	BaseUnit
	PTS time.Duration
	AU  []byte
}

type formatProcessorMPEG4AudioLATM struct {
	udpMaxPayloadSize int
	format            *formats.MPEG4AudioLATM
	encoder           *rtpmpeg4audiolatm.Encoder
	decoder           *rtpmpeg4audiolatm.Decoder
}

func newMPEG4AudioLATM(
	udpMaxPayloadSize int,
	forma *formats.MPEG4AudioLATM,
	generateRTPPackets bool,
	_ logger.Writer,
) (*formatProcessorMPEG4AudioLATM, error) {
	t := &formatProcessorMPEG4AudioLATM{
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

func (t *formatProcessorMPEG4AudioLATM) createEncoder() error {
	t.encoder = &rtpmpeg4audiolatm.Encoder{
		PayloadType: t.format.PayloadTyp,
		Config:      t.format.Config,
	}
	return t.encoder.Init()
}

func (t *formatProcessorMPEG4AudioLATM) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitMPEG4AudioLATM)

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

			au, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg4audiolatm.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.AU = au
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.AU, tunit.PTS)
	if err != nil {
		return err
	}
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorMPEG4AudioLATM) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitMPEG4AudioLATM{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
		},
	}
}
