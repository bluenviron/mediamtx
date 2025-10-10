package codecprocessor //nolint:dupl

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtplpcm"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type lpcm struct {
	RTPMaxPayloadSize  int
	Format             *format.LPCM
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtplpcm.Encoder
	decoder     *rtplpcm.Decoder
	randomStart uint32
}

func (t *lpcm) initialize() error {
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

func (t *lpcm) createEncoder() error {
	t.encoder = &rtplpcm.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
		BitDepth:       t.Format.BitDepth,
		ChannelCount:   t.Format.ChannelCount,
	}
	return t.encoder.Init()
}

func (t *lpcm) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadLPCM))
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *lpcm) ProcessRTPPacket( //nolint:dupl
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

		samples, err := t.decoder.Decode(pkt)
		if err != nil {
			return err
		}

		u.Payload = unit.PayloadLPCM(samples)
	}

	return nil
}
