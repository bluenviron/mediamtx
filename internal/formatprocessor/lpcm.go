package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtplpcm"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type lpcm struct {
	RTPMaxPayloadSize  int
	Format             *format.LPCM
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtplpcm.Encoder
	decoder     *rtplpcm.Decoder
	randomStart uint32
}

func (t *lpcm) initialize() error {
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

func (t *lpcm) createEncoder() error {
	t.encoder = &rtplpcm.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
		BitDepth:       t.Format.BitDepth,
		ChannelCount:   t.Format.ChannelCount,
	}
	return t.encoder.Init()
}

func (t *lpcm) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.LPCM)

	pkts, err := t.encoder.Encode(u.Samples)
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *lpcm) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.LPCM{
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

		samples, err := t.decoder.Decode(pkt)
		if err != nil {
			return nil, err
		}

		u.Samples = samples
	}

	// route packet as is
	return u, nil
}
