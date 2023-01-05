package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpvp9"
	"github.com/pion/rtp"
)

// DataVP9 is a VP9 data unit.
type DataVP9 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	Frame      []byte
}

// GetRTPPackets implements Data.
func (d *DataVP9) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Data.
func (d *DataVP9) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorVP9 struct {
	format  *format.VP9
	encoder *rtpvp9.Encoder
	decoder *rtpvp9.Decoder
}

func newVP9(
	forma *format.VP9,
	allocateEncoder bool,
) (*formatProcessorVP9, error) {
	t := &formatProcessorVP9{
		format: forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorVP9) Process(dat Data, hasNonRTSPReaders bool) error { //nolint:dupl
	tdata := dat.(*DataVP9)

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
				if err == rtpvp9.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tdata.Frame = frame
			tdata.PTS = PTS
		}

		// route packet as is
		return nil
	}

	pkts, err := t.encoder.Encode(tdata.Frame, tdata.PTS)
	if err != nil {
		return err
	}

	tdata.RTPPackets = pkts
	return nil
}
