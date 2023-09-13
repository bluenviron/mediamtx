package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmpeg4audiolatm"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorMPEG4AudioLATM struct {
	udpMaxPayloadSize int
	format            *format.MPEG4AudioLATM
	encoder           *rtpmpeg4audiolatm.Encoder
	decoder           *rtpmpeg4audiolatm.Decoder
}

func newMPEG4AudioLATM(
	udpMaxPayloadSize int,
	forma *format.MPEG4AudioLATM,
	generateRTPPackets bool,
) (*formatProcessorMPEG4AudioLATM, error) {
	t := &formatProcessorMPEG4AudioLATM{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if generateRTPPackets {
		err := t.createEncoder()
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *formatProcessorMPEG4AudioLATM) createEncoder() error {
	t.encoder = &rtpmpeg4audiolatm.Encoder{
		PayloadType: t.format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *formatProcessorMPEG4AudioLATM) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.MPEG4AudioLATM)

	pkts, err := t.encoder.Encode(u.AU)
	if err != nil {
		return err
	}

	ts := uint32(multiplyAndDivide(u.PTS, time.Duration(t.format.ClockRate()), time.Second))
	for _, pkt := range pkts {
		pkt.Timestamp = ts
	}

	u.RTPPackets = pkts

	return nil
}

func (t *formatProcessorMPEG4AudioLATM) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts time.Duration,
	hasNonRTSPReaders bool,
) (Unit, error) {
	u := &unit.MPEG4AudioLATM{
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

		au, err := t.decoder.Decode(pkt)
		if err != nil {
			if err == rtpmpeg4audiolatm.ErrMorePacketsNeeded {
				return u, nil
			}
			return nil, err
		}

		u.AU = au
	}

	// route packet as is
	return u, nil
}
