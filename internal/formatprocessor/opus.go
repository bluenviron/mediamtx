package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpsimpleaudio"
	"github.com/bluenviron/mediacommon/pkg/codecs/opus"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitOpus is a Opus data unit.
type UnitOpus struct {
	BaseUnit
	PTS     time.Duration
	Packets [][]byte
}

type formatProcessorOpus struct {
	udpMaxPayloadSize int
	format            *formats.Opus
	encoder           *rtpsimpleaudio.Encoder
	decoder           *rtpsimpleaudio.Decoder
}

func newOpus(
	udpMaxPayloadSize int,
	forma *formats.Opus,
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
		SampleRate:     48000,
	}
	return t.encoder.Init()
}

func (t *formatProcessorOpus) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitOpus)

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
				t.decoder, err = t.format.CreateDecoder2()
				if err != nil {
					return err
				}
			}

			packet, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				return err
			}

			tunit.Packets = [][]byte{packet}
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	var rtpPackets []*rtp.Packet //nolint:prealloc
	pts := tunit.PTS
	for _, packet := range tunit.Packets {
		pkt, err := t.encoder.Encode(packet, pts)
		if err != nil {
			return err
		}

		rtpPackets = append(rtpPackets, pkt)
		pts += opus.PacketDuration(packet)
	}
	tunit.RTPPackets = rtpPackets

	return nil
}

func (t *formatProcessorOpus) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitOpus{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
		},
	}
}
