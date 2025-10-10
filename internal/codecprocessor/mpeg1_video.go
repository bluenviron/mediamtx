package codecprocessor //nolint:dupl

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpmpeg1video"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// MPEG-1 video related parameters
var (
	MPEG1VideoDefaultConfig = []byte{
		0x00, 0x00, 0x01, 0xb3, 0x78, 0x04, 0x38, 0x35,
		0xff, 0xff, 0xe0, 0x18, 0x00, 0x00, 0x01, 0xb5,
		0x14, 0x4a, 0x00, 0x01, 0x00, 0x00,
	}
)

type mpeg1Video struct {
	RTPMaxPayloadSize  int
	Format             *format.MPEG1Video
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpmpeg1video.Encoder
	decoder     *rtpmpeg1video.Decoder
	randomStart uint32
}

func (t *mpeg1Video) initialize() error {
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

func (t *mpeg1Video) createEncoder() error {
	t.encoder = &rtpmpeg1video.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
	}
	return t.encoder.Init()
}

func (t *mpeg1Video) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	// encode into RTP
	pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadMPEG1Video))
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *mpeg1Video) ProcessRTPPacket( //nolint:dupl
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

		frame, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpmpeg1video.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpmpeg1video.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		u.Payload = unit.PayloadMPEG1Video(frame)
	}

	return nil
}
