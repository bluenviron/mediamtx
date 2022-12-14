package core

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtph264"
	"github.com/aler9/gortsplib/v2/pkg/h264"
	"github.com/pion/rtp"
)

// extract SPS and PPS without decoding RTP packets
func rtpH264ExtractSPSPPS(pkt *rtp.Packet) ([]byte, []byte) {
	if len(pkt.Payload) < 1 {
		return nil, nil
	}

	typ := h264.NALUType(pkt.Payload[0] & 0x1F)

	switch typ {
	case h264.NALUTypeSPS:
		return pkt.Payload, nil

	case h264.NALUTypePPS:
		return nil, pkt.Payload

	case h264.NALUTypeSTAPA:
		payload := pkt.Payload[1:]
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
				return nil, nil
			}

			nalu := payload[:size]
			payload = payload[size:]

			typ = h264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case h264.NALUTypeSPS:
				sps = nalu

			case h264.NALUTypePPS:
				pps = nalu
			}
		}

		return sps, pps

	default:
		return nil, nil
	}
}

type dataH264 struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	nalus      [][]byte
}

func (d *dataH264) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataH264) getNTP() time.Time {
	return d.ntp
}

type formatProcessorH264 struct {
	format *format.H264

	encoder *rtph264.Encoder
	decoder *rtph264.Decoder
}

func newFormatProcessorH264(
	forma *format.H264,
	allocateEncoder bool,
) (*formatProcessorH264, error) {
	t := &formatProcessorH264{
		format: forma,
	}

	if allocateEncoder {
		t.encoder = forma.CreateEncoder()
	}

	return t, nil
}

func (t *formatProcessorH264) updateTrackParametersFromRTPPacket(pkt *rtp.Packet) {
	sps, pps := rtpH264ExtractSPSPPS(pkt)

	if sps != nil && !bytes.Equal(sps, t.format.SafeSPS()) {
		t.format.SafeSetSPS(sps)
	}

	if pps != nil && !bytes.Equal(pps, t.format.SafePPS()) {
		t.format.SafeSetPPS(pps)
	}
}

func (t *formatProcessorH264) updateTrackParametersFromNALUs(nalus [][]byte) {
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS:
			if !bytes.Equal(nalu, t.format.SafeSPS()) {
				t.format.SafeSetSPS(nalu)
			}

		case h264.NALUTypePPS:
			if !bytes.Equal(nalu, t.format.SafePPS()) {
				t.format.SafeSetPPS(nalu)
			}
		}
	}
}

// remux is needed to fix corrupted streams and make streams
// compatible with all protocols.
func (t *formatProcessorH264) remuxNALUs(nalus [][]byte) [][]byte {
	addSPSPPS := false
	n := 0
	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue
		case h264.NALUTypeAccessUnitDelimiter:
			continue
		case h264.NALUTypeIDR:
			// prepend SPS and PPS to the group if there's at least an IDR
			if !addSPSPPS {
				addSPSPPS = true
				n += 2
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	if addSPSPPS {
		filteredNALUs[0] = t.format.SafeSPS()
		filteredNALUs[1] = t.format.SafePPS()
		i = 2
	}

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			// remove since they're automatically added
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			// remove since it is not needed
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *formatProcessorH264) process(dat data, hasNonRTSPReaders bool) error { //nolint:dupl
	tdata := dat.(*dataH264)

	if tdata.rtpPackets != nil {
		pkt := tdata.rtpPackets[0]
		t.updateTrackParametersFromRTPPacket(pkt)

		if t.encoder == nil {
			// remove padding
			pkt.Header.Padding = false
			pkt.PaddingSize = 0

			// RTP packets exceed maximum size: start re-encoding them
			if pkt.MarshalSize() > maxPacketSize {
				v1 := pkt.SSRC
				v2 := pkt.SequenceNumber
				v3 := pkt.Timestamp
				t.encoder = &rtph264.Encoder{
					PayloadType:           pkt.PayloadType,
					SSRC:                  &v1,
					InitialSequenceNumber: &v2,
					InitialTimestamp:      &v3,
					PacketizationMode:     t.format.PacketizationMode,
				}
				t.encoder.Init()
			}
		}

		// decode from RTP
		if hasNonRTSPReaders || t.encoder != nil {
			if t.decoder == nil {
				t.decoder = t.format.CreateDecoder()
			}

			tdata.rtpPackets = nil

			// DecodeUntilMarker() is necessary, otherwise Encode() generates partial groups
			nalus, pts, err := t.decoder.DecodeUntilMarker(pkt)
			if err != nil {
				if err == rtph264.ErrNonStartingPacketAndNoPrevious || err == rtph264.ErrMorePacketsNeeded {
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

	pkts, err := t.encoder.Encode(tdata.nalus, tdata.pts)
	if err != nil {
		return err
	}

	tdata.rtpPackets = pkts
	return nil
}
