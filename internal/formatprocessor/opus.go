package formatprocessor

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpsimpleaudio"
	"github.com/pion/rtp"
)

// DataOpus is a Opus data unit.
type DataOpus struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	Frame      []byte
}

// GetRTPPackets implements Data.
func (d *DataOpus) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Data.
func (d *DataOpus) GetNTP() time.Time {
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

func (t *formatProcessorOpus) Process(dat Data, hasNonRTSPReaders bool) error { //nolint:dupl
	tdata := dat.(*DataOpus)

	if tdata.RTPPackets != nil {
		pkt := tdata.RTPPackets[0]

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

			frame, PTS, err := t.decoder.Decode(pkt)
			if err != nil {
				return err
			}

			tdata.Frame = frame
			tdata.PTS = PTS
		}

		// route packet as is
		return nil
	}

	pkt, err := t.encoder.Encode(tdata.Frame, tdata.PTS)
	if err != nil {
		return err
	}

	tdata.RTPPackets = []*rtp.Packet{pkt}
	return nil
}
