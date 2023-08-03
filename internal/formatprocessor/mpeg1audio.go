package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpmpeg1audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitMPEG1Audio is a MPEG-1/2 Audio data unit.
type UnitMPEG1Audio struct {
	BaseUnit
	PTS    time.Duration
	Frames [][]byte
}

type formatProcessorMPEG1Audio struct {
	udpMaxPayloadSize int
	format            *formats.MPEG1Audio
	encoder           *rtpmpeg1audio.Encoder
	decoder           *rtpmpeg1audio.Decoder
}

func newMPEG1Audio(
	udpMaxPayloadSize int,
	forma *formats.MPEG1Audio,
	generateRTPPackets bool,
	_ logger.Writer,
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

func (t *formatProcessorMPEG1Audio) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitMPEG1Audio)

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
				t.decoder, err = t.format.CreateDecoder2()
				if err != nil {
					return err
				}
			}

			frames, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg1audio.ErrNonStartingPacketAndNoPrevious || err == rtpmpeg1audio.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.Frames = frames
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.Frames, tunit.PTS)
	if err != nil {
		return err
	}
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorMPEG1Audio) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitMPEG1Audio{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
		},
	}
}
