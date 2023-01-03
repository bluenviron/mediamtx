package core //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpvp8"
	"github.com/pion/rtp"
)

type dataVP8 struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	frame      []byte
}

func (d *dataVP8) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataVP8) getNTP() time.Time {
	return d.ntp
}

type formatProcessorVP8 struct {
	format  *format.VP8
	encoder *rtpvp8.Encoder
	decoder *rtpvp8.Decoder
}

func newFormatProcessorVP8(
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

func (t *formatProcessorVP8) process(dat data, hasNonRTSPReaders bool) error { //nolint:dupl
	tdata := dat.(*dataVP8)

	if tdata.rtpPackets != nil {
		pkt := tdata.rtpPackets[0]

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

			tdata.frame = frame
			tdata.pts = pts
		}

		// route packet as is
		return nil
	}

	pkts, err := t.encoder.Encode(tdata.frame, tdata.pts)
	if err != nil {
		return err
	}

	tdata.rtpPackets = pkts
	return nil
}
