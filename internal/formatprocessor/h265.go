package formatprocessor

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h265"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtph265"
	"github.com/pion/rtp"
)

// extract VPS, SPS and PPS without decoding RTP packets
func rtpH265ExtractVPSSPSPPS(pkt *rtp.Packet) ([]byte, []byte, []byte) {
	if len(pkt.Payload) < 2 {
		return nil, nil, nil
	}

	typ := h265.NALUType((pkt.Payload[0] >> 1) & 0b111111)

	switch typ {
	case h265.NALUType_VPS_NUT:
		return pkt.Payload, nil, nil

	case h265.NALUType_SPS_NUT:
		return nil, pkt.Payload, nil

	case h265.NALUType_PPS_NUT:
		return nil, nil, pkt.Payload

	case h265.NALUType_AggregationUnit:
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
			case h265.NALUType_VPS_NUT:
				vps = nalu

			case h265.NALUType_SPS_NUT:
				sps = nalu

			case h265.NALUType_PPS_NUT:
				pps = nalu
			}
		}

		return vps, sps, pps

	default:
		return nil, nil, nil
	}
}

// DataH265 is a H265 data unit.
type DataH265 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	AU         [][]byte
}

// GetRTPPackets implements Data.
func (d *DataH265) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Data.
func (d *DataH265) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorH265 struct {
	format *format.H265

	encoder *rtph265.Encoder
	decoder *rtph265.Decoder
}

func newH265(
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
	for _, nalu := range nalus {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT:
			if !bytes.Equal(nalu, t.format.SafeVPS()) {
				t.format.SafeSetVPS(nalu)
			}

		case h265.NALUType_SPS_NUT:
			if !bytes.Equal(nalu, t.format.SafePPS()) {
				t.format.SafeSetSPS(nalu)
			}

		case h265.NALUType_PPS_NUT:
			if !bytes.Equal(nalu, t.format.SafePPS()) {
				t.format.SafeSetPPS(nalu)
			}
		}
	}
}

func (t *formatProcessorH265) remuxAccessUnit(nalus [][]byte) [][]byte {
	var vps []byte
	var sps []byte
	var pps []byte
	addParameters := false
	n := 0

	for _, nalu := range nalus {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT: // remove parameters
			continue

		case h265.NALUType_AUD_NUT: // remove AUDs
			continue

		// prepend parameters if there's at least a random access unit
		case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
			if !addParameters {
				addParameters = true

				vps = t.format.SafeVPS()
				sps = t.format.SafeSPS()
				pps = t.format.SafePPS()

				if vps != nil && sps != nil && pps != nil {
					n += 3
				}
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	if addParameters && vps != nil && sps != nil && pps != nil {
		filteredNALUs[0] = vps
		filteredNALUs[1] = sps
		filteredNALUs[2] = pps
		i = 3
	}

	for _, nalu := range nalus {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT:
			continue

		case h265.NALUType_AUD_NUT:
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *formatProcessorH265) Process(dat Data, hasNonRTSPReaders bool) error { //nolint:dupl
	tdata := dat.(*DataH265)

	if tdata.RTPPackets != nil {
		pkt := tdata.RTPPackets[0]
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
				t.encoder = &rtph265.Encoder{
					PayloadType:           pkt.PayloadType,
					SSRC:                  &v1,
					InitialSequenceNumber: &v2,
					InitialTimestamp:      &v3,
					MaxDONDiff:            t.format.MaxDONDiff,
				}
				t.encoder.Init()
			}
		}

		// decode from RTP
		if hasNonRTSPReaders || t.encoder != nil {
			if t.decoder == nil {
				t.decoder = t.format.CreateDecoder()
			}

			if t.encoder != nil {
				tdata.RTPPackets = nil
			}

			// DecodeUntilMarker() is necessary, otherwise Encode() generates partial groups
			au, PTS, err := t.decoder.DecodeUntilMarker(pkt)
			if err != nil {
				if err == rtph265.ErrNonStartingPacketAndNoPrevious || err == rtph265.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tdata.AU = au
			tdata.PTS = PTS
			tdata.AU = t.remuxAccessUnit(tdata.AU)
		}

		// route packet as is
		if t.encoder == nil {
			return nil
		}
	} else {
		t.updateTrackParametersFromNALUs(tdata.AU)
		tdata.AU = t.remuxAccessUnit(tdata.AU)
	}

	if len(tdata.AU) != 0 {
		pkts, err := t.encoder.Encode(tdata.AU, tdata.PTS)
		if err != nil {
			return err
		}
		tdata.RTPPackets = pkts
	} else {
		tdata.RTPPackets = nil
	}

	return nil
}
