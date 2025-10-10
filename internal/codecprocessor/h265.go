package codecprocessor

import (
	"bytes"
	"errors"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph265"
	mch265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// H265-related parameters
var (
	H265DefaultVPS = []byte{
		0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x02, 0x20,
		0x00, 0x00, 0x03, 0x00, 0xb0, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x03, 0x00, 0x7b, 0x18, 0xb0, 0x24,
	}

	H265DefaultSPS = []byte{
		0x42, 0x01, 0x01, 0x02, 0x20, 0x00, 0x00, 0x03,
		0x00, 0xb0, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
		0x00, 0x7b, 0xa0, 0x07, 0x82, 0x00, 0x88, 0x7d,
		0xb6, 0x71, 0x8b, 0x92, 0x44, 0x80, 0x53, 0x88,
		0x88, 0x92, 0xcf, 0x24, 0xa6, 0x92, 0x72, 0xc9,
		0x12, 0x49, 0x22, 0xdc, 0x91, 0xaa, 0x48, 0xfc,
		0xa2, 0x23, 0xff, 0x00, 0x01, 0x00, 0x01, 0x6a,
		0x02, 0x02, 0x02, 0x01,
	}

	H265DefaultPPS = []byte{
		0x44, 0x01, 0xc0, 0x25, 0x2f, 0x05, 0x32, 0x40,
	}
)

// extract VPS, SPS and PPS without decoding RTP packets
func rtpH265ExtractParams(payload []byte) ([]byte, []byte, []byte) {
	if len(payload) < 2 {
		return nil, nil, nil
	}

	typ := mch265.NALUType((payload[0] >> 1) & 0b111111)

	switch typ {
	case mch265.NALUType_VPS_NUT:
		return payload, nil, nil

	case mch265.NALUType_SPS_NUT:
		return nil, payload, nil

	case mch265.NALUType_PPS_NUT:
		return nil, nil, payload

	case mch265.NALUType_AggregationUnit:
		payload = payload[2:]
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

			typ = mch265.NALUType((nalu[0] >> 1) & 0b111111)

			switch typ {
			case mch265.NALUType_VPS_NUT:
				vps = nalu

			case mch265.NALUType_SPS_NUT:
				sps = nalu

			case mch265.NALUType_PPS_NUT:
				pps = nalu
			}
		}

		return vps, sps, pps

	default:
		return nil, nil, nil
	}
}

type h265 struct {
	RTPMaxPayloadSize  int
	Format             *format.H265
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtph265.Encoder
	decoder     *rtph265.Decoder
	randomStart uint32
}

func (t *h265) initialize() error {
	if t.GenerateRTPPackets {
		err := t.createEncoder(nil, nil)
		if err != nil {
			return err
		}

		t.randomStart, err = randUint32()
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *h265) createEncoder(
	ssrc *uint32,
	initialSequenceNumber *uint16,
) error {
	t.encoder = &rtph265.Encoder{
		PayloadMaxSize:        t.RTPMaxPayloadSize,
		PayloadType:           t.Format.PayloadTyp,
		SSRC:                  ssrc,
		InitialSequenceNumber: initialSequenceNumber,
		MaxDONDiff:            t.Format.MaxDONDiff,
	}
	return t.encoder.Init()
}

func (t *h265) updateTrackParametersFromRTPPacket(payload []byte) {
	vps, sps, pps := rtpH265ExtractParams(payload)

	if (vps != nil && !bytes.Equal(vps, t.Format.VPS)) ||
		(sps != nil && !bytes.Equal(sps, t.Format.SPS)) ||
		(pps != nil && !bytes.Equal(pps, t.Format.PPS)) {
		if vps == nil {
			vps = t.Format.VPS
		}
		if sps == nil {
			sps = t.Format.SPS
		}
		if pps == nil {
			pps = t.Format.PPS
		}
		t.Format.SafeSetParams(vps, sps, pps)
	}
}

func (t *h265) updateTrackParametersFromAU(au unit.PayloadH265) {
	vps := t.Format.VPS
	sps := t.Format.SPS
	pps := t.Format.PPS
	update := false

	for _, nalu := range au {
		typ := mch265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case mch265.NALUType_VPS_NUT:
			if !bytes.Equal(nalu, t.Format.VPS) {
				vps = nalu
				update = true
			}

		case mch265.NALUType_SPS_NUT:
			if !bytes.Equal(nalu, t.Format.SPS) {
				sps = nalu
				update = true
			}

		case mch265.NALUType_PPS_NUT:
			if !bytes.Equal(nalu, t.Format.PPS) {
				pps = nalu
				update = true
			}
		}
	}

	if update {
		t.Format.SafeSetParams(vps, sps, pps)
	}
}

func (t *h265) remuxAccessUnit(au unit.PayloadH265) unit.PayloadH265 {
	isKeyFrame := false
	n := 0

	for _, nalu := range au {
		typ := mch265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case mch265.NALUType_VPS_NUT, mch265.NALUType_SPS_NUT, mch265.NALUType_PPS_NUT:
			continue

		case mch265.NALUType_AUD_NUT:
			continue

		case mch265.NALUType_IDR_W_RADL, mch265.NALUType_IDR_N_LP, mch265.NALUType_CRA_NUT:
			if !isKeyFrame {
				isKeyFrame = true

				// prepend parameters
				if t.Format.VPS != nil && t.Format.SPS != nil && t.Format.PPS != nil {
					n += 3
				}
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredAU := make([][]byte, n)
	i := 0

	if isKeyFrame && t.Format.VPS != nil && t.Format.SPS != nil && t.Format.PPS != nil {
		filteredAU[0] = t.Format.VPS
		filteredAU[1] = t.Format.SPS
		filteredAU[2] = t.Format.PPS
		i = 3
	}

	for _, nalu := range au {
		typ := mch265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case mch265.NALUType_VPS_NUT, mch265.NALUType_SPS_NUT, mch265.NALUType_PPS_NUT:
			continue

		case mch265.NALUType_AUD_NUT:
			continue
		}

		filteredAU[i] = nalu
		i++
	}

	return filteredAU
}

func (t *h265) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	t.updateTrackParametersFromAU(u.Payload.(unit.PayloadH265))
	u.Payload = t.remuxAccessUnit(u.Payload.(unit.PayloadH265))

	if !u.NilPayload() {
		pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadH265))
		if err != nil {
			return err
		}
		u.RTPPackets = pkts

		for _, pkt := range u.RTPPackets {
			pkt.Timestamp += t.randomStart + uint32(u.PTS)
		}
	}

	return nil
}

func (t *h265) ProcessRTPPacket( //nolint:dupl
	u *unit.Unit,
	hasNonRTSPReaders bool,
) error {
	pkt := u.RTPPackets[0]

	t.updateTrackParametersFromRTPPacket(pkt.Payload)

	if t.encoder == nil {
		// remove padding
		pkt.Padding = false
		pkt.PaddingSize = 0

		// RTP packets exceed maximum size: start re-encoding them
		if len(pkt.Payload) > t.RTPMaxPayloadSize {
			t.Parent.Log(logger.Info, "RTP packets are too big, remuxing them into smaller ones")

			v1 := pkt.SSRC
			v2 := pkt.SequenceNumber
			err := t.createEncoder(&v1, &v2)
			if err != nil {
				return err
			}
		}
	}

	// decode from RTP
	if hasNonRTSPReaders || t.decoder != nil || t.encoder != nil {
		if t.decoder == nil {
			var err error
			t.decoder, err = t.Format.CreateDecoder()
			if err != nil {
				return err
			}
		}

		au, err := t.decoder.Decode(pkt)

		if t.encoder != nil {
			u.RTPPackets = nil
		}

		if err != nil {
			if errors.Is(err, rtph265.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtph265.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		u.Payload = t.remuxAccessUnit(au)
	}

	// route packet as is
	if t.encoder == nil {
		return nil
	}

	// encode into RTP
	if !u.NilPayload() {
		pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadH265))
		if err != nil {
			return err
		}
		u.RTPPackets = pkts

		for _, newPKT := range u.RTPPackets {
			newPKT.Timestamp = pkt.Timestamp
		}
	}

	return nil
}
