package formatprocessor

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpsimpleaudio"
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
	format  *format.Opus
	encoder *rtpsimpleaudio.Encoder
	decoder *rtpsimpleaudio.Decoder
}

func newOpus(
	forma *format.Opus,
	allocateEncoder bool,
) (*formatProcessorOpus, error) {
	t := &formatProcessorOpus{
		format: forma,
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

		if pkt.MarshalSize() > maxPacketSize {
			return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
				pkt.MarshalSize(), maxPacketSize)
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
