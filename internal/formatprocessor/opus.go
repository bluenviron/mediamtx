package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpsimpleaudio"
	"github.com/bluenviron/mediacommon/pkg/codecs/opus"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorOpus struct {
	udpMaxPayloadSize int
	format            *format.Opus
	encoder           *rtpsimpleaudio.Encoder
	decoder           *rtpsimpleaudio.Decoder
}

func newOpus(
	udpMaxPayloadSize int,
	forma *format.Opus,
	generateRTPPackets bool,
	_ logger.Writer,
) (*formatProcessorOpus, error) {
	t := &formatProcessorOpus{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if generateRTPPackets {
		err := t.createEncoder()
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *formatProcessorOpus) createEncoder() error {
	t.encoder = &rtpsimpleaudio.Encoder{
		PayloadMaxSize: t.udpMaxPayloadSize - 12,
		PayloadType:    t.format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *formatProcessorOpus) Process(u unit.Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := u.(*unit.Opus)

	if tunit.RTPPackets != nil {
		pkt := tunit.RTPPackets[0]

		// remove padding
		pkt.Header.Padding = false
		pkt.PaddingSize = 0

		if pkt.MarshalSize() > t.udpMaxPayloadSize {
			return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
				pkt.MarshalSize(), t.udpMaxPayloadSize)
		}

		// decode from RTP
		if hasNonRTSPReaders || t.decoder != nil {
			if t.decoder == nil {
				var err error
				t.decoder, err = t.format.CreateDecoder()
				if err != nil {
					return err
				}
			}

			packet, err := t.decoder.Decode(pkt)
			if err != nil {
				return err
			}

			tunit.Packets = [][]byte{packet}
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	var rtpPackets []*rtp.Packet //nolint:prealloc
	pts := tunit.PTS
	for _, packet := range tunit.Packets {
		pkt, err := t.encoder.Encode(packet)
		if err != nil {
			return err
		}
		setTimestamp([]*rtp.Packet{pkt}, tunit.RTPPackets, t.format.ClockRate(), pts)
		rtpPackets = append(rtpPackets, pkt)
		pts += opus.PacketDuration(packet)
	}

	tunit.RTPPackets = rtpPackets

	return nil
}

func (t *formatProcessorOpus) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time, pts time.Duration) Unit {
	return &unit.Opus{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}
}
