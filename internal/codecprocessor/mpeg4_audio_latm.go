package codecprocessor

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpfragmented"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmpeg4audio"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type mpeg4AudioLATM struct {
	RTPMaxPayloadSize  int
	Format             *format.MPEG4AudioLATM
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpfragmented.Encoder
	decoder     *rtpfragmented.Decoder
	randomStart uint32
}

func (t *mpeg4AudioLATM) initialize() error {
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

func (t *mpeg4AudioLATM) createEncoder() error {
	t.encoder = &rtpfragmented.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *mpeg4AudioLATM) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadMPEG4AudioLATM))
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *mpeg4AudioLATM) ProcessRTPPacket( //nolint:dupl
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

		el, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpmpeg4audio.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		u.Payload = unit.PayloadMPEG4AudioLATM(el)
	}

	return nil
}
