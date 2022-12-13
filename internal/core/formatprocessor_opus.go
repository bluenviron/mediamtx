package core

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpsimpleaudio"
	"github.com/pion/rtp"
)

type dataOpus struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	au         []byte
}

func (d *dataOpus) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataOpus) getNTP() time.Time {
	return d.ntp
}

type formatProcessorOpus struct {
	format  *format.Opus
	encoder *rtpsimpleaudio.Encoder
	decoder *rtpsimpleaudio.Decoder
}

func newFormatProcessorOpus(
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

func (t *formatProcessorOpus) generateRTPPackets(tdata *dataOpus) error {
	pkt, err := t.encoder.Encode(tdata.au, tdata.pts)
	if err != nil {
		return err
	}

	tdata.rtpPackets = []*rtp.Packet{pkt}
	return nil
}

func (t *formatProcessorOpus) process(dat data, hasNonRTSPReaders bool) error {
	tdata := dat.(*dataOpus)

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

			au, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				return err
			}

			tdata.au = au
			tdata.pts = pts
		}

		// route packet as is
		return nil
	}

	return t.generateRTPPackets(tdata)
}
