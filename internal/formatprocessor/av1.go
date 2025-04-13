package formatprocessor //nolint:dupl

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpav1"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

// AV1-related parameters
var (
	AV1DefaultSequenceHeader = []byte{
		8, 0, 0, 0, 66, 167, 191, 228, 96, 13, 0, 64,
	}
)

type av1 struct {
	UDPMaxPayloadSize  int
	Format             *format.AV1
	GenerateRTPPackets bool

	encoder     *rtpav1.Encoder
	decoder     *rtpav1.Decoder
	randomStart uint32
}

func (t *av1) initialize() error {
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

func (t *av1) createEncoder() error {
	t.encoder = &rtpav1.Encoder{
		PayloadMaxSize: t.UDPMaxPayloadSize - 12,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *av1) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.AV1)

	pkts, err := t.encoder.Encode(u.TU)
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *av1) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.AV1{
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

		tu, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpav1.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpav1.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.TU = tu
	}

	// route packet as is
	return u, nil
}
