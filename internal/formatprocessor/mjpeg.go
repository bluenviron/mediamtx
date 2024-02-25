package formatprocessor //nolint:dupl

import (
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmjpeg"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorMJPEG struct {
	udpMaxPayloadSize int
	format            *format.MJPEG
	timeEncoder       *rtptime.Encoder
	encoder           *rtpmjpeg.Encoder
	decoder           *rtpmjpeg.Decoder
}

func newMJPEG(
	udpMaxPayloadSize int,
	forma *format.MJPEG,
	generateRTPPackets bool,
) (*formatProcessorMJPEG, error) {
	t := &formatProcessorMJPEG{
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

func (t *formatProcessorMJPEG) createEncoder() error {
	t.encoder = &rtpmjpeg.Encoder{
		PayloadMaxSize: t.udpMaxPayloadSize - 12,
	}
	return t.encoder.Init()
}

func (t *formatProcessorMJPEG) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.MJPEG)

	// encode into RTP
	pkts, err := t.encoder.Encode(u.Frame)
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

func (t *formatProcessorMJPEG) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts time.Duration,
	hasNonRTSPReaders bool,
) (Unit, error) {
	u := &unit.MJPEG{
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

		frame, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpmjpeg.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpmjpeg.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.Frame = frame
	}

	// route packet as is
	return u, nil
}
