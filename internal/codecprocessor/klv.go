package codecprocessor

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpklv"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type klv struct {
	RTPMaxPayloadSize  int
	Format             *format.KLV
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpklv.Encoder
	decoder     *rtpklv.Decoder
	randomStart uint32
}

func (t *klv) initialize() error {
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

func (t *klv) createEncoder() error {
	t.encoder = &rtpklv.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *klv) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	if t.encoder == nil {
		err := t.createEncoder()
		if err != nil {
			return err
		}
	}

	pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadKLV))
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *klv) ProcessRTPPacket( //nolint:dupl
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

		un, err := t.decoder.Decode(pkt)
		if err != nil {
			return err
		}

		u.Payload = unit.PayloadKLV(un)
	}

	return nil
}
