package formatprocessor

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpac3"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorAC3 struct {
	udpMaxPayloadSize int
	format            *format.AC3
	timeEncoder       *rtptime.Encoder
	encoder           *rtpac3.Encoder
	decoder           *rtpac3.Decoder
}

func newAC3(
	udpMaxPayloadSize int,
	forma *format.AC3,
	generateRTPPackets bool,
) (*formatProcessorAC3, error) {
	t := &formatProcessorAC3{
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

func (t *formatProcessorAC3) createEncoder() error {
	t.encoder = &rtpac3.Encoder{
		PayloadType: t.format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *formatProcessorAC3) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.AC3)

	pkts, err := t.encoder.Encode(u.Frames)
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	ts := t.timeEncoder.Encode(u.PTS)
	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += ts
	}

	return nil
}

func (t *formatProcessorAC3) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts time.Duration,
	hasNonRTSPReaders bool,
) (Unit, error) {
	u := &unit.AC3{
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

		frames, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpac3.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpac3.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.Frames = frames
	}

	// route packet as is
	return u, nil
}
