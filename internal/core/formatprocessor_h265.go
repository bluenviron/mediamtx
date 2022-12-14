package core

import (
	"bytes"
	"fmt"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtph265"
	"github.com/aler9/gortsplib/v2/pkg/h265"
	"github.com/pion/rtp"
)

// extract VPS, SPS and PPS without decoding RTP packets
func rtpH265ExtractVPSSPSPPS(pkt *rtp.Packet) ([]byte, []byte, []byte) {
	if len(pkt.Payload) < 2 {
		return nil, nil, nil
	}

	typ := h265.NALUType((pkt.Payload[0] >> 1) & 0b111111)

	switch typ {
	case h265.NALUTypeVPS:
		return pkt.Payload, nil, nil

	case h265.NALUTypeSPS:
		return nil, pkt.Payload, nil

	case h265.NALUTypePPS:
		return nil, nil, pkt.Payload

	case h265.NALUTypeAggregationUnit:
		payload := pkt.Payload[2:]
		var vps []byte
		var sps []byte
		var pps []byte

		for len(payload) > 0 {
			if len(payload) < 2 {
				break
			}

			size := uint16(payload[0])<<8 | uint16(payload[1])
			payload = payload[2:]

			if size == 0 {
				break
			}

			if int(size) > len(payload) {
				return nil, nil, nil
			}

			nalu := payload[:size]
			payload = payload[size:]

			typ = h265.NALUType((pkt.Payload[0] >> 1) & 0b111111)

			switch typ {
			case h265.NALUTypeVPS:
				vps = nalu

			case h265.NALUTypeSPS:
				sps = nalu

			case h265.NALUTypePPS:
				pps = nalu
			}
		}

		return vps, sps, pps

	default:
		return nil, nil, nil
	}
}

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
	vps, sps, pps := rtpH265ExtractVPSSPSPPS(pkt)

	if vps != nil && !bytes.Equal(vps, t.format.SafeVPS()) {
		t.format.SafeSetVPS(vps)
	}

	if sps != nil && !bytes.Equal(sps, t.format.SafeSPS()) {
		t.format.SafeSetSPS(sps)
	}

	if pps != nil && !bytes.Equal(pps, t.format.SafePPS()) {
		t.format.SafeSetPPS(pps)
	}
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
