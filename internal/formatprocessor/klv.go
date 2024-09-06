package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpsimpleaudio"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorKLV struct {
	udpMaxPayloadSize int
	format            *format.KLV
	timeEncoder       *rtptime.Encoder
	encoder           *rtpsimpleaudio.Encoder
	decoder           *rtpsimpleaudio.Decoder
}

func newKLV(
	udpMaxPayloadSize int,
	forma *format.KLV,
	generateRTPPackets bool,
) (*formatProcessorKLV, error) {
	t := &formatProcessorKLV{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if generateRTPPackets {
		err := t.createEncoder()
		if err != nil {
			return nil, err
		}

		t.timeEncoder = &rtptime.Encoder{
			ClockRate: forma.ClockRate(),
		}
		err = t.timeEncoder.Initialize()
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *formatProcessorKLV) createEncoder() error {
	t.encoder = &rtpsimpleaudio.Encoder{
		PayloadMaxSize: t.udpMaxPayloadSize - 12,
		PayloadType:    t.format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *formatProcessorKLV) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.KLV)

	var rtpPackets []*rtp.Packet
	pts := u.PTS

	if u.Packets != nil {
		// ensure the format processor's encoder is initialized
		if t.encoder == nil {
			err := t.createEncoder()
			if err != nil {
				return err
			}
		}
		pkt, err := t.encoder.Encode(u.Packets)
		if err != nil {
			return err
		}

		ts := t.timeEncoder.Encode(pts)
		for _, pkt := range u.RTPPackets {
			pkt.Timestamp += ts
		}

		rtpPackets = append(rtpPackets, pkt)
	}

	u.RTPPackets = rtpPackets

	return nil
}

func (t *formatProcessorKLV) ProcessRTPPacket(
	pkt *rtp.Packet,
	ntp time.Time,
	pts time.Duration,
	hasNonRTSPReaders bool,
) (Unit, error) {
	u := &unit.KLV{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}

	// remove padding
	pkt.Header.Padding = false
	pkt.PaddingSize = 0

	if pkt.MarshalSize() > t.udpMaxPayloadSize {
		return nil, fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
			pkt.MarshalSize(), t.udpMaxPayloadSize)
	}

	// decode from RTP
	if hasNonRTSPReaders || t.decoder != nil {
		if t.decoder == nil {
			var err error
			t.decoder, err = t.format.CreateDecoder()
			if err != nil {
				return nil, err
			}
		}

		packet, err := t.decoder.Decode(pkt)
		if err != nil {
			return nil, err
		}

		u.Packets = packet
	}

	// route packet as is
	return u, nil
}
