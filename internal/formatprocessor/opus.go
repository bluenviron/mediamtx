package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpsimpleaudio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorOpus struct {
	UDPMaxPayloadSize  int
	Format             *format.Opus
	GenerateRTPPackets bool

	encoder     *rtpsimpleaudio.Encoder
	decoder     *rtpsimpleaudio.Decoder
	randomStart uint32
}

func (t *formatProcessorOpus) initialize() error {
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

func (t *formatProcessorOpus) createEncoder() error {
	t.encoder = &rtpsimpleaudio.Encoder{
		PayloadMaxSize: t.UDPMaxPayloadSize - 12,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *formatProcessorOpus) ProcessUnit(uu unit.Unit) error { //nolint:dupl
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
		pts += int64(opus.PacketDuration(packet)) * int64(t.Format.ClockRate()) / int64(time.Second)
	}

	u.RTPPackets = rtpPackets

	return nil
}

func (t *formatProcessorOpus) ProcessRTPPacket(
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
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > t.UDPMaxPayloadSize {
		return nil, fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
			pkt.MarshalSize(), t.UDPMaxPayloadSize)
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
