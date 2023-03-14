package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpvp8"
	"github.com/pion/rtp"
)

// UnitVP8 is a VP8 data unit.
type UnitVP8 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	Frame      []byte
}

// GetRTPPackets implements Unit.
func (d *UnitVP8) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitVP8) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorVP8 struct {
	format  *format.VP8
	encoder *rtpvp8.Encoder
	decoder *rtpvp8.Decoder
}

func newVP8(
	forma *format.VP8,
	allocateEncoder bool,
) (*formatProcessorVP8, error) {
	t := &formatProcessorVP8{
		format: forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorVP8) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitVP8)

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
				if err == rtpvp8.ErrMorePacketsNeeded {
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

	pkts, err := t.encoder.Encode(tunit.Frame, tunit.PTS)
	if err != nil {
		return err
	}

	tunit.RTPPackets = pkts
	return nil
}
