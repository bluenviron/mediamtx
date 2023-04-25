package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpvp9"
	"github.com/pion/rtp"
)

// UnitVP9 is a VP9 data unit.
type UnitVP9 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	Frame      []byte
}

// GetRTPPackets implements Unit.
func (d *UnitVP9) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitVP9) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorVP9 struct {
	udpMaxPayloadSize int
	format            *formats.VP9
	encoder           *rtpvp9.Encoder
	decoder           *rtpvp9.Decoder
}

func newVP9(
	udpMaxPayloadSize int,
	forma *formats.VP9,
	allocateEncoder bool,
) (*formatProcessorVP9, error) {
	t := &formatProcessorVP9{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if allocateEncoder {
		t.encoder = &rtpvp9.Encoder{
			PayloadMaxSize: t.udpMaxPayloadSize - 12,
			PayloadType:    forma.PayloadTyp,
		}
		t.encoder.Init()
	}

	return t, nil
}

func (t *formatProcessorVP9) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitVP9)

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
				if err == rtpvp9.ErrMorePacketsNeeded {
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
