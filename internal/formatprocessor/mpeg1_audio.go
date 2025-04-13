package formatprocessor //nolint:dupl

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmpeg1audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type mpeg1Audio struct {
	UDPMaxPayloadSize  int
	Format             *format.MPEG1Audio
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpmpeg1audio.Encoder
	decoder     *rtpmpeg1audio.Decoder
	randomStart uint32
}

func (t *mpeg1Audio) initialize() error {
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

func (t *mpeg1Audio) createEncoder() error {
	t.encoder = &rtpmpeg1audio.Encoder{
		PayloadMaxSize: t.UDPMaxPayloadSize - 12,
	}
	return t.encoder.Init()
}

func (t *mpeg1Audio) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.MPEG1Audio)

	pkts, err := t.encoder.Encode(u.Frames)
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *mpeg1Audio) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.MPEG1Audio{
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

		frames, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpmpeg1audio.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpmpeg1audio.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.Frames = frames
	}

	// route packet as is
	return u, nil
}
