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
	udpMaxPayloadSize int
	format            *format.MPEG4Audio
	encoder           *rtpmpeg4audio.Encoder
	decoder           *rtpmpeg4audio.Decoder
	randomStart       uint32
}

func newMPEG4Audio(
	udpMaxPayloadSize int,
	forma *format.MPEG4Audio,
	generateRTPPackets bool,
) (*formatProcessorMPEG4Audio, error) {
	t := &formatProcessorMPEG4Audio{
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

func (t *formatProcessorMPEG4Audio) createEncoder() error {
	t.encoder = &rtpmpeg4audio.Encoder{
		PayloadMaxSize:   t.udpMaxPayloadSize - 12,
		PayloadType:      t.format.PayloadTyp,
		SizeLength:       t.format.SizeLength,
		IndexLength:      t.format.IndexLength,
		IndexDeltaLength: t.format.IndexDeltaLength,
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
