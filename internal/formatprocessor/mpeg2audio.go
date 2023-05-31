package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpmpeg2audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitMPEG2Audio is a MPEG-1/2 Audio data unit.
type UnitMPEG2Audio struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	Frames     [][]byte
}

// GetRTPPackets implements Unit.
func (d *UnitMPEG2Audio) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitMPEG2Audio) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorMPEG2Audio struct {
	udpMaxPayloadSize int
	format            *formats.MPEG2Audio
	encoder           *rtpmpeg2audio.Encoder
	decoder           *rtpmpeg2audio.Decoder
}

func newMPEG2Audio(
	udpMaxPayloadSize int,
	forma *formats.MPEG2Audio,
	generateRTPPackets bool,
	_ logger.Writer,
) (*formatProcessorMPEG2Audio, error) {
	t := &formatProcessorMPEG2Audio{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if generateRTPPackets {
		t.encoder = &rtpmpeg2audio.Encoder{
			PayloadMaxSize: t.udpMaxPayloadSize - 12,
		}
		t.encoder.Init()
	}

	return t, nil
}

func (t *formatProcessorMPEG2Audio) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitMPEG2Audio)

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
				t.decoder = t.format.CreateDecoder()
			}

			frames, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg2audio.ErrNonStartingPacketAndNoPrevious || err == rtpmpeg2audio.ErrMorePacketsNeeded {
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

func (t *formatProcessorMPEG2Audio) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitMPEG2Audio{
		RTPPackets: []*rtp.Packet{pkt},
		NTP:        ntp,
	}
}
