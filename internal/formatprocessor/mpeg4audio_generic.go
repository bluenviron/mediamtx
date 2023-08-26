package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmpeg4audio"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatProcessorMPEG4AudioGeneric struct {
	udpMaxPayloadSize int
	format            *format.MPEG4Audio
	encoder           *rtpmpeg4audio.Encoder
	decoder           *rtpmpeg4audio.Decoder
}

func newMPEG4AudioGeneric(
	udpMaxPayloadSize int,
	forma *format.MPEG4Audio,
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
		SizeLength:       t.format.SizeLength,
		IndexLength:      t.format.IndexLength,
		IndexDeltaLength: t.format.IndexDeltaLength,
	}
	return t.encoder.Init()
}

func (t *formatProcessorMPEG4AudioGeneric) Process(u unit.Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := u.(*unit.MPEG4AudioGeneric)

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
				t.decoder, err = t.format.CreateDecoder()
				if err != nil {
					return err
				}
			}

			aus, err := t.decoder.Decode(pkt)
			if err != nil {
				if err == rtpmpeg4audio.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.AUs = aus
		}

		// route packet as is
		return nil
	}

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.AUs)
	if err != nil {
		return err
	}
	setTimestamp(pkts, tunit.RTPPackets, t.format.ClockRate(), tunit.PTS)
	tunit.RTPPackets = pkts

	return nil
}

func (t *formatProcessorMPEG4AudioGeneric) UnitForRTPPacket(pkt *rtp.Packet, ntp time.Time, pts time.Duration) Unit {
	return &unit.MPEG4AudioGeneric{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}
}
