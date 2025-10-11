package codecprocessor

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpsimpleaudio"
	mcopus "github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type opus struct {
	RTPMaxPayloadSize  int
	Format             *format.Opus
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpsimpleaudio.Encoder
	decoder     *rtpsimpleaudio.Decoder
	randomStart uint32
}

func (t *opus) initialize() error {
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

func (t *opus) createEncoder() error {
	t.encoder = &rtpsimpleaudio.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *opus) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	var rtpPackets []*rtp.Packet //nolint:prealloc
	pts := u.PTS

	for _, packet := range u.Payload.(unit.PayloadOpus) {
		pkt, err := t.encoder.Encode(packet)
		if err != nil {
			return err
		}

		pkt.Timestamp += t.randomStart + uint32(pts)

		rtpPackets = append(rtpPackets, pkt)
		pts += mcopus.PacketDuration2(packet)
	}

	u.RTPPackets = rtpPackets

	return nil
}

func (t *opus) ProcessRTPPacket(
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

		packet, err := t.decoder.Decode(pkt)
		if err != nil {
			return err
		}

		u.Payload = unit.PayloadOpus{packet}
	}

	return nil
}
