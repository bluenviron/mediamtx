package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpvp8"
	"github.com/pion/rtp"
)

// DataVP8 is a VP8 data unit.
type DataVP8 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	Frame      []byte
}

// GetRTPPackets implements Data.
func (d *DataVP8) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Data.
func (d *DataVP8) GetNTP() time.Time {
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

func (t *formatProcessorVP8) Process(dat Data, hasNonRTSPReaders bool) error { //nolint:dupl
	tdata := dat.(*DataVP8)

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
				if err == rtpvp8.ErrMorePacketsNeeded {
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
