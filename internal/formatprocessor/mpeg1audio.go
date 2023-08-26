package formatprocessor //nolint:dupl

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmpeg1audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorMPEG1Audio struct {
	udpMaxPayloadSize int
	format            *format.MPEG1Audio
	encoder           *rtpmpeg1audio.Encoder
	decoder           *rtpmpeg1audio.Decoder
}

func newMPEG1Audio(
	udpMaxPayloadSize int,
	forma *format.MPEG1Audio,
	generateRTPPackets bool,
) (*formatProcessorMPEG1Audio, error) {
	t := &formatProcessorMPEG1Audio{
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

func (t *formatProcessorMPEG1Audio) createEncoder() error {
	t.encoder = &rtpmpeg1audio.Encoder{
		PayloadMaxSize: t.udpMaxPayloadSize - 12,
	}
	return t.encoder.Init()
}

func (t *formatProcessorMPEG1Audio) Process(u unit.Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := u.(*unit.MPEG1Audio)

	if tunit.RTPPackets != nil {
		pkt := tunit.RTPPackets[0]

		// remove padding
		pkt.Header.Padding = false
		pkt.PaddingSize = 0

		if pkt.MarshalSize() > t.udpMaxPayloadSize {
			return fmt.Errorf("payload size (%d) is greater than maximum allowed (%d)",
				pkt.MarshalSize(), t.udpMaxPayloadSize)
		}

		// decode from RTP
		if hasNonRTSPReaders || t.decoder != nil {
			if t.decoder == nil {
				var err error
				t.decoder, err = t.format.CreateDecoder()
				if err != nil {
					return err
				}
			}

			frames, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg1audio.ErrNonStartingPacketAndNoPrevious || err == rtpmpeg1audio.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.Frames = frames
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.Frames)
	if err != nil {
		return err
	}
	setTimestamp(pkts, tunit.RTPPackets, t.format.ClockRate(), tunit.PTS)
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorMPEG1Audio) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time, pts time.Duration) Unit {
	return &unit.MPEG1Audio{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}
}
