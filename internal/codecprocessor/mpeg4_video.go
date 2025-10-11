package codecprocessor //nolint:dupl

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtpfragmented"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// MPEG-4 video related parameters
var (
	MPEG4VideoDefaultConfig = []byte{
		0x00, 0x00, 0x01, 0xb0, 0x01, 0x00, 0x00, 0x01,
		0xb5, 0x89, 0x13, 0x00, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x01, 0x20, 0x00, 0xc4, 0x8d, 0x88, 0x00,
		0xf5, 0x3c, 0x04, 0x87, 0x14, 0x63, 0x00, 0x00,
		0x01, 0xb2, 0x4c, 0x61, 0x76, 0x63, 0x35, 0x38,
		0x2e, 0x31, 0x33, 0x34, 0x2e, 0x31, 0x30, 0x30,
	}
)

type mpeg4Video struct {
	RTPMaxPayloadSize  int
	Format             *format.MPEG4Video
	GenerateRTPPackets bool
	Parent             logger.Writer

	encoder     *rtpfragmented.Encoder
	decoder     *rtpfragmented.Decoder
	randomStart uint32
}

func (t *mpeg4Video) initialize() error {
	if t.GenerateRTPPackets {
		err := t.createEncoder()
		if err != nil {
			return err
		}

		t.randomStart, err = randUint32()
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *mpeg4Video) createEncoder() error {
	t.encoder = &rtpfragmented.Encoder{
		PayloadMaxSize: t.RTPMaxPayloadSize,
		PayloadType:    t.Format.PayloadTyp,
	}
	return t.encoder.Init()
}

func (t *mpeg4Video) updateTrackParameters(frame unit.PayloadMPEG4Video) {
	if bytes.HasPrefix(frame, []byte{0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode)}) {
		end := bytes.Index(frame[4:], []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})
		if end < 0 {
			return
		}
		conf := frame[:end+4]

		if !bytes.Equal(conf, t.Format.Config) {
			t.Format.SafeSetParams(conf)
		}
	}
}

func (t *mpeg4Video) remuxFrame(frame unit.PayloadMPEG4Video) unit.PayloadMPEG4Video {
	// remove config
	if bytes.HasPrefix(frame, []byte{0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode)}) {
		end := bytes.Index(frame[4:], []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})
		if end >= 0 {
			frame = frame[end+4:]
		}
	}

	// add config
	if bytes.Contains(frame, []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)}) {
		f := make([]byte, len(t.Format.Config)+len(frame))
		n := copy(f, t.Format.Config)
		copy(f[n:], frame)
		frame = f
	}

	if len(frame) == 0 {
		return nil
	}

	return frame
}

func (t *mpeg4Video) ProcessUnit(u *unit.Unit) error { //nolint:dupl
	t.updateTrackParameters(u.Payload.(unit.PayloadMPEG4Video))
	u.Payload = t.remuxFrame(u.Payload.(unit.PayloadMPEG4Video))

	if !u.NilPayload() {
		pkts, err := t.encoder.Encode(u.Payload.(unit.PayloadMPEG4Video))
		if err != nil {
			return err
		}
		u.RTPPackets = pkts

		for _, pkt := range u.RTPPackets {
			pkt.Timestamp += t.randomStart + uint32(u.PTS)
		}
	}

	return nil
}

func (t *mpeg4Video) ProcessRTPPacket( //nolint:dupl
	u *unit.Unit,
	hasNonRTSPReaders bool,
) error {
	pkt := u.RTPPackets[0]

	t.updateTrackParameters(pkt.Payload)

	// remove padding
	pkt.Padding = false
	pkt.PaddingSize = 0

	if len(pkt.Payload) > t.RTPMaxPayloadSize {
		return fmt.Errorf("RTP payload size (%d) is greater than maximum allowed (%d)",
			len(pkt.Payload), t.RTPMaxPayloadSize)
	}

	// decode from RTP
	if hasNonRTSPReaders || t.decoder != nil {
		if t.decoder == nil {
			var err error
			t.decoder, err = t.Format.CreateDecoder()
			if err != nil {
				return err
			}
		}

		frame, err := t.decoder.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtpfragmented.ErrMorePacketsNeeded) {
				return nil
			}
			return err
		}

		u.Payload = t.remuxFrame(frame)
	}

	return nil
}
