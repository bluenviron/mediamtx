package formatprocessor

import (
	"bytes"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtph264"
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

// DataH264 is a H264 data unit.
type DataH264 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	AU         [][]byte
}

// GetRTPPackets implements Data.
func (d *DataH264) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Data.
func (d *DataH264) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorH264 struct {
	format *format.H264

	encoder *rtph264.Encoder
	decoder *rtph264.Decoder
}

func newH264(
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

func (t *formatProcessorH264) remuxAccessUnit(nalus [][]byte) [][]byte {
	var sps []byte
	var pps []byte
	addParameters := false
	n := 0

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS: // remove parameters
			continue

		case h264.NALUTypeAccessUnitDelimiter: // remove AUDs
			continue

		case h264.NALUTypeIDR: // prepend parameters if there's at least an IDR
			if !addParameters {
				addParameters = true
				sps = t.format.SafeSPS()
				pps = t.format.SafePPS()

				if sps != nil && pps != nil {
					n += 2
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

	if addParameters && sps != nil && pps != nil {
		filteredNALUs[0] = sps
		filteredNALUs[1] = pps
		i = 2
	}

	for _, nalu := range nalus {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *formatProcessorH264) Process(dat Data, hasNonRTSPReaders bool) error { //nolint:dupl
	tdata := dat.(*DataH264)

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

			if t.encoder != nil {
				tdata.RTPPackets = nil
			}

			// DecodeUntilMarker() is necessary, otherwise Encode() generates partial groups
			au, PTS, err := t.decoder.DecodeUntilMarker(pkt)
			if err != nil {
				if err == rtph264.ErrNonStartingPacketAndNoPrevious || err == rtph264.ErrMorePacketsNeeded {
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
