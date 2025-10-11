package codecprocessor

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpac3"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type ac3 struct {
	RTPMaxPayloadSize  int
	Format             *format.AC3
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpac3.Encoder
	decoder     *rtpac3.Decoder
	randomStart uint32
}

func (t *ac3) initialize() error {
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

func (t *ac3) createEncoder() error {
	t.encoder = &rtpac3.Encoder{
		PayloadType: t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *ac3) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadAC3))
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *ac3) ProcessRTPPacket( //nolint:dupl
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

		frames, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpac3.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpac3.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		u.Payload = unit.PayloadAC3(frames)
	}

	return nil
}
