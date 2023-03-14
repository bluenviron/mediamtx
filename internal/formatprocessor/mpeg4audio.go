package formatprocessor

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpmpeg4audio"
	"github.com/pion/rtp"
)

// UnitMPEG4Audio is a MPEG4-audio data unit.
type UnitMPEG4Audio struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	AUs        [][]byte
}

// GetRTPPackets implements Unit.
func (d *UnitMPEG4Audio) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitMPEG4Audio) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorMPEG4Audio struct {
	format  *format.MPEG4Audio
	encoder *rtpmpeg4audio.Encoder
	decoder *rtpmpeg4audio.Decoder
}

func newMPEG4Audio(
	forma *format.MPEG4Audio,
	allocateEncoder bool,
) (*formatProcessorMPEG4Audio, error) {
	t := &formatProcessorMPEG4Audio{
		format: forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorMPEG4Audio) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitMPEG4Audio)

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

			aus, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg4audio.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.AUs = aus
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	pkts, err := t.encoder.Encode(tunit.AUs, tunit.PTS)
	if err != nil {
		return err
	}

	tunit.RTPPackets = pkts
	return nil
}
