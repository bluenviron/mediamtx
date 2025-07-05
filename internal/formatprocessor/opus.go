package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpsimpleaudio"
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

func (t *opus) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.Opus)

	var rtpPackets []*rtp.Packet //nolint:prealloc
	pts := u.PTS

	for _, packet := range u.Packets {
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
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.Opus{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}

	// remove padding
	pkt.Padding = false
	pkt.PaddingSize = 0

	if len(pkt.Payload) > t.RTPMaxPayloadSize {
		return nil, fmt.Errorf("RTP payload size (%d) is greater than maximum allowed (%d)",
			len(pkt.Payload), t.RTPMaxPayloadSize)
	}

	// decode from RTP
	if hasNonRTSPReaders || t.decoder != nil {
		if t.decoder == nil {
			var err error
			t.decoder, err = t.Format.CreateDecoder()
			if err != nil {
				return nil, err
			}
		}

		packet, err := t.decoder.Decode(pkt)
		if err != nil {
			return nil, err
		}

		u.Packets = [][]byte{packet}
	}

	// route packet as is
	return u, nil
}
