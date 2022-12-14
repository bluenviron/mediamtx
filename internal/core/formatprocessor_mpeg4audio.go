package core

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpmpeg4audio"
	"github.com/pion/rtp"
)

type dataMPEG4Audio struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	aus        [][]byte
}

func (d *dataMPEG4Audio) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataMPEG4Audio) getNTP() time.Time {
	return d.ntp
}

type formatProcessorMPEG4Audio struct {
	format  *format.MPEG4Audio
	encoder *rtpmpeg4audio.Encoder
	decoder *rtpmpeg4audio.Decoder
}

func newFormatProcessorMPEG4Audio(
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

func (t *formatProcessorMPEG4Audio) process(dat data, hasNonRTSPReaders bool) error {
	tdata := dat.(*dataMPEG4Audio)

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

			aus, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg4audio.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tdata.aus = aus
			tdata.pts = pts
		}

		// route packet as is
		return nil
	}

	pkts, err := t.encoder.Encode(tdata.aus, tdata.pts)
	if err != nil {
		return err
	}

	tdata.rtpPackets = pkts
	return nil
}
