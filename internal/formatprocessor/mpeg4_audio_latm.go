package formatprocessor

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpfragmented"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmpeg4audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type mpeg4AudioLATM struct {
	RTPMaxPayloadSize  int
	Format             *format.MPEG4AudioLATM
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpfragmented.Encoder
	decoder     *rtpfragmented.Decoder
	randomStart uint32
}

func (t *mpeg4AudioLATM) initialize() error {
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

func (t *mpeg4AudioLATM) createEncoder() error {
	t.encoder = &rtpfragmented.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *mpeg4AudioLATM) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.MPEG4AudioLATM)

	pkts, err := t.encoder.Encode(u.Element)
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *mpeg4AudioLATM) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.MPEG4AudioLATM{
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

		el, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpmpeg4audio.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.Element = el
	}

	// route packet as is
	return u, nil
}
