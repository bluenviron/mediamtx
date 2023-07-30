package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpmpeg4audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// UnitMPEG4AudioGeneric is a MPEG-4 Audio data unit.
type UnitMPEG4AudioGeneric struct {
	BaseUnit
	PTS time.Duration
	AUs [][]byte
}

type formatProcessorMPEG4AudioGeneric struct {
	udpMaxPayloadSize int
	format            *formats.MPEG4Audio
	encoder           *rtpmpeg4audio.Encoder
	decoder           *rtpmpeg4audio.Decoder
}

func newMPEG4AudioGeneric(
	udpMaxPayloadSize int,
	forma *formats.MPEG4Audio,
	generateRTPPackets bool,
	_ logger.Writer,
) (*formatProcessorMPEG4AudioGeneric, error) {
	t := &formatProcessorMPEG4AudioGeneric{
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

func (t *formatProcessorMPEG4AudioGeneric) createEncoder() error {
	t.encoder = &rtpmpeg4audio.Encoder{
		PayloadMaxSize:   t.udpMaxPayloadSize - 12,
		PayloadType:      t.format.PayloadTyp,
		SampleRate:       t.format.Config.SampleRate,
		SizeLength:       t.format.SizeLength,
		IndexLength:      t.format.IndexLength,
		IndexDeltaLength: t.format.IndexDeltaLength,
	}
	return t.encoder.Init()
}

func (t *formatProcessorMPEG4AudioGeneric) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitMPEG4AudioGeneric)

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
		if hasNonRTSPReaders || t.decoder != nil || true {
			if t.decoder == nil {
				var err error
				t.decoder, err = t.format.CreateDecoder2()
				if err != nil {
					return err
				}
			}

			aus, pts, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg4audio.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.AUs = aus
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.AUs, tunit.PTS)
	if err != nil {
		return err
	}
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorMPEG4AudioGeneric) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time) Unit {
	return &UnitMPEG4AudioGeneric{
		BaseUnit: BaseUnit{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
		},
	}
}
