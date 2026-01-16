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


// ExtractMPEG4Resolution extracts width and height from MPEG-4 Video config
func ExtractMPEG4Resolution(config []byte) (int, int) {
	// MPEG-4 Video Object Layer (VOL) parsing for resolution
	// Look for VOL start code 0x00 0x00 0x01 0x20
	if len(config) < 20 {
		return 0, 0
	}
	for i := 0; i < len(config)-4; i++ {
		if config[i] == 0x00 && config[i+1] == 0x00 && config[i+2] == 0x01 && config[i+3] == 0x20 {
			// VOL header starts after start code
			data := config[i+4:]
			if len(data) < 10 {
				continue
			}
			// Skip vol_id (4 bits), random_accessible_vol (1), video_object_type_indication (8)
			// is_object_layer_identifier (1)
			bitPos := 0
			// vol_id: 4 bits
			bitPos += 4
			// random_accessible_vol: 1 bit
			bitPos += 1
			// video_object_type_indication: 8 bits
			bitPos += 8
			// is_object_layer_identifier: 1 bit
			isObjectLayer := getBit(data, bitPos)
			bitPos += 1
			if isObjectLayer {
				// video_object_layer_verid: 4 bits
				bitPos += 4
				// video_object_layer_priority: 3 bits
				bitPos += 3
			}
			// aspect_ratio_info: 4 bits
			aspectRatio := getBits(data, bitPos, 4)
			bitPos += 4
			if aspectRatio == 15 { // extended_PAR
				// par_width: 8 bits
				bitPos += 8
				// par_height: 8 bits
				bitPos += 8
			}
			// vol_control_parameters: 1 bit
			volControl := getBit(data, bitPos)
			bitPos += 1
			if volControl {
				// chroma_format: 2 bits
				bitPos += 2
				// low_delay: 1 bit
				bitPos += 1
				// vbv_parameters: 1 bit
				vbv := getBit(data, bitPos)
				bitPos += 1
				if vbv {
					// bit_rate: 15 bits
					bitPos += 15
					// buffer_size: 15 bits
					bitPos += 15
					// vbv_occupancy: 15 bits
					bitPos += 15
				}
			}
			// video_object_layer_shape: 2 bits
			shape := getBits(data, bitPos, 2)
			bitPos += 2
			if shape == 3 { // gray_scale
				// video_object_layer_shape_extension: 4 bits
				bitPos += 4
			}
			// marker_bit: 1 bit
			bitPos += 1
			// vop_time_increment_resolution: 16 bits
			bitPos += 16
			// marker_bit: 1 bit
			bitPos += 1
			// fixed_vop_rate: 1 bit
			fixedVop := getBit(data, bitPos)
			bitPos += 1
			if fixedVop {
				// fixed_vop_time_increment: variable bits
				// skip for now
			}
			// marker_bit: 1 bit
			bitPos += 1
			// video_object_layer_width: 13 bits
			width := getBits(data, bitPos, 13)
			bitPos += 13
			// marker_bit: 1 bit
			bitPos += 1
			// video_object_layer_height: 13 bits
			height := getBits(data, bitPos, 13)
			return int(width), int(height)
		}
	}
	return 0, 0
}

func getBit(data []byte, bitPos int) bool {
	byteIndex := bitPos / 8
	bitIndex := 7 - (bitPos % 8)
	if byteIndex >= len(data) {
		return false
	}
	return (data[byteIndex] & (1 << bitIndex)) != 0
}

func getBits(data []byte, bitPos int, numBits int) uint32 {
	var val uint32
	for i := 0; i < numBits; i++ {
		if getBit(data, bitPos+i) {
			val |= 1 << (numBits - 1 - i)
		}
	}
	return val
}