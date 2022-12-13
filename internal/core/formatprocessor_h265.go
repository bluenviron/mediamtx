package core

import (
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtph265"
	"github.com/pion/rtp"
)

type dataH265 struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	nalus      [][]byte
}

func (d *dataH265) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataH265) getNTP() time.Time {
	return d.ntp
}

type formatProcessorH265 struct {
	format *format.H265

	encoder *rtph265.Encoder
	decoder *rtph265.Decoder
}

func newFormatProcessorH265(
	forma *format.H265,
	allocateEncoder bool,
) (*formatProcessorH265, error) {
	t := &formatProcessorH265{
		format: forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorH265) updateTrackParametersFromRTPPacket(pkt *rtp.Packet) {
	// TODO: extract VPS, SPS, PPS and set them into the track
}

func (t *formatProcessorH265) updateTrackParametersFromNALUs(nalus [][]byte) {
	// TODO: extract VPS, SPS, PPS and set them into the track
}

func (t *formatProcessorH265) remuxNALUs(nalus [][]byte) [][]byte {
	// TODO: add VPS, SPS, PPS before IDRs
	return nalus
}

func (t *formatProcessorH265) generateRTPPackets(tdata *dataH265) error {
	pkts, err := t.encoder.Encode(tdata.nalus, tdata.pts)
	if err != nil {
		return err
	}

	tdata.rtpPackets = pkts
	return nil
}

func (t *formatProcessorH265) process(dat data, hasNonRTSPReaders bool) error {
	tdata := dat.(*dataH265)

	if tdata.rtpPackets != nil {
		pkt := tdata.rtpPackets[0]
		t.updateTrackParametersFromRTPPacket(pkt)

		if t.encoder == nil {
			// remove padding
			pkt.Header.Padding = false
			pkt.PaddingSize = 0

			// TODO: re-encode if oversized instead of printing errors
			if pkt.MarshalSize() > maxPacketSize {
				return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
					pkt.MarshalSize(), maxPacketSize)
			}
		}

		// decode from RTP
		if hasNonRTSPReaders || t.encoder != nil {
			if t.decoder == nil {
				t.decoder = t.format.CreateDecoder()
			}

			nalus, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtph265.ErrNonStartingPacketAndNoPrevious || err == rtph265.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tdata.nalus = nalus
			tdata.pts = pts

			tdata.nalus = t.remuxNALUs(tdata.nalus)
		}

		// route packet as is
		if t.encoder == nil {
			return nil
		}
	} else {
		t.updateTrackParametersFromNALUs(tdata.nalus)
		tdata.nalus = t.remuxNALUs(tdata.nalus)
	}

	return t.generateRTPPackets(tdata)
}
