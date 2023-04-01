package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpsimpleaudio"
	"github.com/pion/rtp"
)

// UnitOpus is a Opus data unit.
type UnitOpus struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	Frame      []byte
}

// GetRTPPackets implements Unit.
func (d *UnitOpus) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitOpus) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorOpus struct {
	udpMaxPayloadSize int
	format            *formats.Opus
	encoder           *rtpsimpleaudio.Encoder
	decoder           *rtpsimpleaudio.Decoder
}

func newOpus(
	udpMaxPayloadSize int,
	forma *formats.Opus,
	allocateEncoder bool,
) (*formatProcessorOpus, error) {
	t := &formatProcessorOpus{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorOpus) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitOpus)

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
		if hasNonRTSPReaders {
			if t.decoder == nil {
				t.decoder = t.format.CreateDecoder()
			}

			frame, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				return err
			}

			tunit.Frame = frame
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	pkt, err := t.encoder.Encode(tunit.Frame, tunit.PTS)
	if err != nil {
		return err
	}

	tunit.RTPPackets = []*rtp.Packet{pkt}
	return nil
}
