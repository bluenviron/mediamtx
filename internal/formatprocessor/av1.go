package formatprocessor

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtpav1"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/pion/rtp"

	"github.com/aler9/mediamtx/internal/logger"
)

// UnitAV1 is an AV1 data unit.
type UnitAV1 struct {
	RTPPackets []*rtp.Packet
	NTP        time.Time
	PTS        time.Duration
	OBUs       [][]byte
}

// GetRTPPackets implements Unit.
func (d *UnitAV1) GetRTPPackets() []*rtp.Packet {
	return d.RTPPackets
}

// GetNTP implements Unit.
func (d *UnitAV1) GetNTP() time.Time {
	return d.NTP
}

type formatProcessorAV1 struct {
	udpMaxPayloadSize int
	format            *formats.AV1
	log               logger.Writer

	encoder              *rtpav1.Encoder
	decoder              *rtpav1.Decoder
	lastKeyFrameReceived time.Time
}

func newAV1(
	udpMaxPayloadSize int,
	forma *formats.AV1,
	generateRTPPackets bool,
	log logger.Writer,
) (*formatProcessorAV1, error) {
	t := &formatProcessorAV1{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
		log:               log,
	}

	if generateRTPPackets {
		t.encoder = &rtpav1.Encoder{
			PayloadMaxSize: t.udpMaxPayloadSize - 12,
		}
		t.encoder.Init()
		t.lastKeyFrameReceived = time.Now()
	}

	return t, nil
}

func (t *formatProcessorAV1) checkKeyFrameInterval(containsKeyFrame bool) {
	if containsKeyFrame {
		t.lastKeyFrameReceived = time.Now()
	} else {
		now := time.Now()
		if now.Sub(t.lastKeyFrameReceived) >= maxKeyFrameInterval {
			t.lastKeyFrameReceived = now
			t.log.Log(logger.Warn, "no AV1 key frames received in %v, stream can't be decoded", maxKeyFrameInterval)
		}
	}
}

func (t *formatProcessorAV1) checkOBUs(obus [][]byte) {
	containsKeyFrame, _ := av1.ContainsKeyFrame(obus)
	t.checkKeyFrameInterval(containsKeyFrame)
}

func (t *formatProcessorAV1) Process(unit Unit, hasNonRTSPReaders bool) error { //nolint:dupl
	tunit := unit.(*UnitAV1)

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
		if hasNonRTSPReaders {
			if t.decoder == nil {
				t.decoder = t.format.CreateDecoder()
				t.lastKeyFrameReceived = time.Now()
			}

			// DecodeUntilMarker() is necessary, otherwise Encode() generates partial groups
			obus, pts, err := t.decoder.DecodeUntilMarker(pkt)
			if err != nil {
				if err == rtpav1.ErrNonStartingPacketAndNoPrevious || err == rtpav1.ErrMorePacketsNeeded {
					return nil
				}
				return err
			}

			tunit.OBUs = obus
			t.checkOBUs(obus)
			tunit.PTS = pts
		}

		// route packet as is
		return nil
	}

	t.checkOBUs(tunit.OBUs)

	// encode into RTP
	pkts, err := t.encoder.Encode(tunit.OBUs, tunit.PTS)
	if err != nil {
		return err
	}
	tunit.RTPPackets = pkts

	return nil
}
