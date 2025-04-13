package formatprocessor //nolint:dupl

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpvp8"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type vp8 struct {
	UDPMaxPayloadSize  int
	Format             *format.VP8
	GenerateRTPPackets bool

	encoder     *rtpvp8.Encoder
	decoder     *rtpvp8.Decoder
	randomStart uint32
}

func (t *vp8) initialize() error {
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

func (t *vp8) createEncoder() error {
	t.encoder = &rtpvp8.Encoder{
		PayloadMaxSize: t.UDPMaxPayloadSize - 12,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *vp8) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.VP8)

	pkts, err := t.encoder.Encode(u.Frame)
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *vp8) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.VP8{
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

		frame, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpvp8.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpvp8.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.Frame = frame
	}

	// route packet as is
	return u, nil
}
