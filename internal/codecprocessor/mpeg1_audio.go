package codecprocessor //nolint:dupl

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmpeg1audio"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type mpeg1Audio struct {
	RTPMaxPayloadSize  int
	Format             *format.MPEG1Audio
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpmpeg1audio.Encoder
	decoder     *rtpmpeg1audio.Decoder
	randomStart uint32
}

func (t *mpeg1Audio) initialize() error {
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

func (t *mpeg1Audio) createEncoder() error {
	t.encoder = &rtpmpeg1audio.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
	}
	return t.encoder.Init()
}

func (t *mpeg1Audio) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadMPEG1Audio))
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *mpeg1Audio) ProcessRTPPacket( //nolint:dupl
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
			if errors.Is(err, rtpmpeg1audio.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpmpeg1audio.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		u.Payload = unit.PayloadMPEG1Audio(frames)
	}

	return nil
}
