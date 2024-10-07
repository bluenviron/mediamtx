package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtplpcm"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorLPCM struct {
	udpMaxPayloadSize int
	format            *format.LPCM
	encoder           *rtplpcm.Encoder
	decoder           *rtplpcm.Decoder
	randomStart       uint32
}

func newLPCM(
	udpMaxPayloadSize int,
	forma *format.LPCM,
	generateRTPPackets bool,
) (*formatProcessorLPCM, error) {
	t := &formatProcessorLPCM{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if generateRTPPackets {
		err := t.createEncoder()
		if err != nil {
			return nil, err
		}

		t.randomStart, err = randUint32()
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *formatProcessorLPCM) createEncoder() error {
	t.encoder = &rtplpcm.Encoder{
		PayloadMaxSize: t.udpMaxPayloadSize - 12,
		PayloadType:    t.format.PayloadTyp,
		BitDepth:       t.format.BitDepth,
		ChannelCount:   t.format.ChannelCount,
	}
	return t.encoder.Init()
}

func (t *formatProcessorLPCM) ProcessUnit(uu unit.Unit) error { //nolint:dupl
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

func (t *formatProcessorLPCM) ProcessRTPPacket( //nolint:dupl
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

		samples, err := t.decoder.Decode(pkt)
		if err != nil {
			return nil, err
		}

		u.Samples = samples
	}

	// route packet as is
	return u, nil
}
