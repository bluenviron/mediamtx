package formatprocessor

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmpeg4audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorMPEG4Audio struct {
	UDPMaxPayloadSize  int
	Format             *format.MPEG4Audio
	GenerateRTPPackets bool

	encoder     *rtpmpeg4audio.Encoder
	decoder     *rtpmpeg4audio.Decoder
	randomStart uint32
}

func (t *formatProcessorMPEG4Audio) initialize() error {
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

func (t *formatProcessorMPEG4Audio) createEncoder() error {
	t.encoder = &rtpmpeg4audio.Encoder{
		PayloadMaxSize:   t.UDPMaxPayloadSize - 12,
		PayloadType:      t.Format.PayloadTyp,
		SizeLength:       t.Format.SizeLength,
		IndexLength:      t.Format.IndexLength,
		IndexDeltaLength: t.Format.IndexDeltaLength,
	}
	return t.encoder.Init()
}

func (t *formatProcessorMPEG4Audio) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.MPEG4Audio)

	pkts, err := t.encoder.Encode(u.AUs)
	if err != nil {
		return err
	}
	u.RTPPackets = pkts

	for _, pkt := range u.RTPPackets {
		pkt.Timestamp += t.randomStart + uint32(u.PTS)
	}

	return nil
}

func (t *formatProcessorMPEG4Audio) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.MPEG4Audio{
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

		aus, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpmpeg4audio.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.AUs = aus
	}

	// route packet as is
	return u, nil
}
