package codecprocessor //nolint:dupl

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpav1"
	mcav1 "github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type av1 struct {
	RTPMaxPayloadSize  int
	Format             *format.AV1
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpav1.Encoder
	decoder     *rtpav1.Decoder
	randomStart uint32
}

func (t *av1) initialize() error {
	if t.GenerateRTPPackets {
		err := t.createEncoder()
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

func (t *av1) createEncoder() error {
	t.encoder = &rtpav1.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *av1) remuxTemporalUnit(tu unit.PayloadAV1) unit.PayloadAV1 {
	n := 0

	for _, obu := range tu {
		typ := mcav1.OBUType((obu[0] >> 3) & 0b1111)

		if typ == mcav1.OBUTypeTemporalDelimiter {
			continue
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredTU := make([][]byte, n)
	i := 0

	for _, obu := range tu {
		typ := mcav1.OBUType((obu[0] >> 3) & 0b1111)

		if typ == mcav1.OBUTypeTemporalDelimiter {
			continue
		}

		filteredTU[i] = obu
		i++
	}

	return filteredTU
}

func (t *av1) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	u.Payload = t.remuxTemporalUnit(u.Payload.(unit.PayloadAV1))

	pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadAV1))
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *av1) ProcessRTPPacket( //nolint:dupl
	u *unit.Unit,
	hasNonRTSPReaders bool,
) error {
	pkt := u.RTPPackets[0]

	// remove padding
	pkt.Padding = false
	pkt.PaddingSize = 0

	if len(pkt.Payload) > t.RTPMaxPayloadSize {
		return fmt.Errorf("RTP payload size (%d) is greater than maximum allowed (%d)",
			len(pkt.Payload), t.RTPMaxPayloadSize)
	}

	// decode from RTP
	if hasNonRTSPReaders || t.decoder != nil {
		if t.decoder == nil {
			var err error
			t.decoder, err = t.Format.CreateDecoder()
			if err != nil {
				return err
			}
		}

		tu, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpav1.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpav1.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		u.Payload = t.remuxTemporalUnit(tu)
	}

	return nil
}
