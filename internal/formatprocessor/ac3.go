package formatprocessor

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpac3"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func randUint32() (uint32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

type formatProcessorAC3 struct {
	udpMaxPayloadSize int
	format            *format.AC3
	encoder           *rtpac3.Encoder
	decoder           *rtpac3.Decoder
	randomStart       uint32
}

func newAC3(
	udpMaxPayloadSize int,
	forma *format.AC3,
	generateRTPPackets bool,
) (*formatProcessorAC3, error) {
	t := &formatProcessorAC3{
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

func (t *formatProcessorAC3) createEncoder() error {
	t.encoder = &rtpac3.Encoder{
		PayloadType: t.format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *formatProcessorAC3) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.AC3)

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

func (t *formatProcessorAC3) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.AC3{
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

		frames, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpac3.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtpac3.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.Frames = frames
	}

	// route packet as is
	return u, nil
}
