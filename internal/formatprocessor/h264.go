package formatprocessor

import (
	"bytes"
	"errors"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	mch264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

// H264-related parameters
var (
	H264DefaultSPS = []byte{ // 1920x1080 baseline
		0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
		0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
		0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
	}

	H264DefaultPPS = []byte{0x08, 0x06, 0x07, 0x08}
)

// extract SPS and PPS without decoding RTP packets
func rtpH264ExtractParams(payload []byte) ([]byte, []byte) {
	if len(payload) < 1 {
		return nil, nil
	}

	typ := mch264.NALUType(payload[0] & 0x1F)

	switch typ {
	case mch264.NALUTypeSPS:
		return payload, nil

	case mch264.NALUTypePPS:
		return nil, payload

	case mch264.NALUTypeSTAPA:
		payload = payload[1:]
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

			typ = mch264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case mch264.NALUTypeSPS:
				sps = nalu

			case mch264.NALUTypePPS:
				pps = nalu
			}
		}

		return sps, pps

	default:
		return nil, nil
	}
}

type h264 struct {
	UDPMaxPayloadSize  int
	Format             *format.H264
	GenerateRTPPackets bool

	encoder     *rtph264.Encoder
	decoder     *rtph264.Decoder
	randomStart uint32
}

func (t *h264) initialize() error {
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

func (t *h264) createEncoder(
	ssrc *uint32,
	initialSequenceNumber *uint16,
) error {
	t.encoder = &rtph264.Encoder{
		PayloadMaxSize:        t.UDPMaxPayloadSize - 12,
		PayloadType:           t.Format.PayloadTyp,
		SSRC:                  ssrc,
		InitialSequenceNumber: initialSequenceNumber,
		PacketizationMode:     t.Format.PacketizationMode,
	}
	return t.encoder.Init()
}

func (t *h264) updateTrackParametersFromRTPPacket(payload []byte) {
	sps, pps := rtpH264ExtractParams(payload)

	if (sps != nil && !bytes.Equal(sps, t.Format.SPS)) ||
		(pps != nil && !bytes.Equal(pps, t.Format.PPS)) {
		if sps == nil {
			sps = t.Format.SPS
		}
		if pps == nil {
			pps = t.Format.PPS
		}
		t.Format.SafeSetParams(sps, pps)
	}
}

func (t *h264) updateTrackParametersFromAU(au [][]byte) {
	sps := t.Format.SPS
	pps := t.Format.PPS
	update := false

	for _, nalu := range au {
		typ := mch264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case mch264.NALUTypeSPS:
			if !bytes.Equal(nalu, sps) {
				sps = nalu
				update = true
			}

		case mch264.NALUTypePPS:
			if !bytes.Equal(nalu, pps) {
				pps = nalu
				update = true
			}
		}
	}

	if update {
		t.Format.SafeSetParams(sps, pps)
	}
}

func (t *h264) remuxAccessUnit(au [][]byte) [][]byte {
	isKeyFrame := false
	n := 0

	for _, nalu := range au {
		typ := mch264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case mch264.NALUTypeSPS, mch264.NALUTypePPS: // parameters: remove
			continue

		case mch264.NALUTypeAccessUnitDelimiter: // AUD: remove
			continue

		case mch264.NALUTypeIDR: // key frame
			if !isKeyFrame {
				isKeyFrame = true

				// prepend parameters
				if t.Format.SPS != nil && t.Format.PPS != nil {
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

	if isKeyFrame && t.Format.SPS != nil && t.Format.PPS != nil {
		filteredNALUs[0] = t.Format.SPS
		filteredNALUs[1] = t.Format.PPS
		i = 2
	}

	for _, nalu := range au {
		typ := mch264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case mch264.NALUTypeSPS, mch264.NALUTypePPS:
			continue

		case mch264.NALUTypeAccessUnitDelimiter:
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *h264) ProcessUnit(uu unit.Unit) error {
	u := uu.(*unit.H264)

	t.updateTrackParametersFromAU(u.AU)
	u.AU = t.remuxAccessUnit(u.AU)

	if u.AU != nil {
		pkts, err := t.encoder.Encode(u.AU)
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

func (t *h264) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.H264{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}

	t.updateTrackParametersFromRTPPacket(pkt.Payload)

	if t.encoder == nil {
		// remove padding
		pkt.Header.Padding = false
		pkt.PaddingSize = 0

		// RTP packets exceed maximum size: start re-encoding them
		if pkt.MarshalSize() > t.UDPMaxPayloadSize {
			v1 := pkt.SSRC
			v2 := pkt.SequenceNumber
			err := t.createEncoder(&v1, &v2)
			if err != nil {
				return nil, err
			}
		}
	}

	// decode from RTP
	if hasNonRTSPReaders || t.decoder != nil || t.encoder != nil {
		if t.decoder == nil {
			var err error
			t.decoder, err = t.Format.CreateDecoder()
			if err != nil {
				return nil, err
			}
		}

		au, err := t.decoder.Decode(pkt)

		if t.encoder != nil {
			u.RTPPackets = nil
		}

		if err != nil {
			if errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtph264.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.AU = t.remuxAccessUnit(au)
	}

	// route packet as is
	if t.encoder == nil {
		return u, nil
	}

	// encode into RTP
	if len(u.AU) != 0 {
		pkts, err := t.encoder.Encode(u.AU)
		if err != nil {
			return nil, err
		}
		u.RTPPackets = pkts

		for _, newPKT := range u.RTPPackets {
			newPKT.Timestamp = pkt.Timestamp
		}
	}

	return u, nil
}
