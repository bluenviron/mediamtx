package stream

import (
	"bytes"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"

	"github.com/bluenviron/mediamtx/internal/unit"
)

// prototype of a function that remuxes a unit payload.
// payload is assumed to be non-nil and to be of the same type as the format.
type unitRemuxer func(format.Format, int64, unit.Payload) unit.Payload

func unitRemuxerAV1(_ format.Format, _ int64, payload unit.Payload) unit.Payload {
	tu := payload.(unit.PayloadAV1)

	n := 0

	for _, obu := range tu {
		typ := av1.OBUType((obu[0] >> 3) & 0b1111)

		if typ == av1.OBUTypeTemporalDelimiter {
			continue
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredTU := make([][]byte, n)
	i := 0

	for _, obu := range tu {
		typ := av1.OBUType((obu[0] >> 3) & 0b1111)

		if typ == av1.OBUTypeTemporalDelimiter {
			continue
		}

		filteredTU[i] = obu
		i++
	}

	return unit.PayloadAV1(filteredTU)
}

// unitRemuxerH265 returns a unitRemuxer that, in addition to stripping
// parameter sets and AUDs, holds back access units that carry no picture
// data (e.g. a standalone prefix-SEI access unit) and prepends them to the
// next access unit that shares their PTS and does contain a picture.
func unitRemuxerH265() unitRemuxer {
	var pending [][]byte
	var pendingPTS int64

	return func(forma format.Format, pts int64, payload unit.Payload) unit.Payload {
		formatH265 := forma.(*format.H265)
		au := payload.(unit.PayloadH265)

		hasPicture := false
		isKeyFrame := false

		for _, nalu := range au {
			switch h265.NALUType((nalu[0] >> 1) & 0b111111) {
			case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT,
				h265.NALUType_AUD_NUT, h265.NALUType_PREFIX_SEI_NUT, h265.NALUType_SUFFIX_SEI_NUT:

			case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
				hasPicture = true
				isKeyFrame = true

			default:
				hasPicture = true
			}
		}

		if !hasPicture {
			var kept [][]byte
			for _, nalu := range au {
				switch h265.NALUType((nalu[0] >> 1) & 0b111111) {
				case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT, h265.NALUType_AUD_NUT:

				// a solitary suffix SEI belongs to a picture that has already
				// been emitted, and cannot be reattached to it
				case h265.NALUType_SUFFIX_SEI_NUT:

				default:
					kept = append(kept, nalu)
				}
			}
			if kept != nil {
				pending, pendingPTS = kept, pts
			}
			return unit.PayloadH265(nil)
		}

		var prefix [][]byte
		if pending != nil && pendingPTS == pts {
			prefix = pending
		}
		pending = nil

		out := make([][]byte, 0, len(prefix)+len(au)+3)

		if isKeyFrame && formatH265.VPS != nil && formatH265.SPS != nil && formatH265.PPS != nil {
			out = append(out, formatH265.VPS, formatH265.SPS, formatH265.PPS)
		}
		out = append(out, prefix...)

		for _, nalu := range au {
			switch h265.NALUType((nalu[0] >> 1) & 0b111111) {
			case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT, h265.NALUType_AUD_NUT:
				continue
			}
			out = append(out, nalu)
		}

		return unit.PayloadH265(out)
	}
}

// unitRemuxerH264 returns a unitRemuxer that, in addition to stripping
// parameter sets and AUDs, holds back access units that carry no VCL NALU
// (e.g. the standalone SEI access unit some DJI drones send ahead of every
// picture) and prepends them to the next access unit that shares their PTS
// and does contain a picture.
func unitRemuxerH264() unitRemuxer {
	var pending [][]byte
	var pendingPTS int64

	return func(forma format.Format, pts int64, payload unit.Payload) unit.Payload {
		formatH264 := forma.(*format.H264)
		au := payload.(unit.PayloadH264)

		hasPicture := false
		isKeyFrame := false

		for _, nalu := range au {
			switch h264.NALUType(nalu[0] & 0x1F) {
			case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter, h264.NALUTypeSEI:

			case h264.NALUTypeIDR:
				hasPicture = true
				isKeyFrame = true

			default:
				hasPicture = true
			}
		}

		if !hasPicture {
			var kept [][]byte
			for _, nalu := range au {
				switch h264.NALUType(nalu[0] & 0x1F) {
				case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:

				default:
					kept = append(kept, nalu)
				}
			}
			if kept != nil {
				pending, pendingPTS = kept, pts
			}
			return unit.PayloadH264(nil)
		}

		var prefix [][]byte
		if pending != nil && pendingPTS == pts {
			prefix = pending
		}
		pending = nil

		out := make([][]byte, 0, len(prefix)+len(au)+2)

		if isKeyFrame && formatH264.SPS != nil && formatH264.PPS != nil {
			out = append(out, formatH264.SPS, formatH264.PPS)
		}
		out = append(out, prefix...)

		for _, nalu := range au {
			switch h264.NALUType(nalu[0] & 0x1F) {
			case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
				continue
			}
			out = append(out, nalu)
		}

		return unit.PayloadH264(out)
	}
}

func unitRemuxerMPEG4Video(forma format.Format, _ int64, payload unit.Payload) unit.Payload {
	formatMPEG4Video := forma.(*format.MPEG4Video)
	frame := payload.(unit.PayloadMPEG4Video)

	// remove config
	if bytes.HasPrefix(frame, []byte{0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode)}) {
		end := bytes.Index(frame[4:], []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})
		if end >= 0 {
			frame = frame[end+4:]
		}
	}

	// add config
	if bytes.Contains(frame, []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)}) {
		f := make([]byte, len(formatMPEG4Video.Config)+len(frame))
		n := copy(f, formatMPEG4Video.Config)
		copy(f[n:], frame)
		frame = f
	}

	if len(frame) == 0 {
		return unit.PayloadMPEG4Video(nil)
	}

	return frame
}

func newUnitRemuxer(forma format.Format) unitRemuxer {
	switch forma.(type) {
	case *format.AV1:
		return unitRemuxerAV1

	case *format.H265:
		return unitRemuxerH265()

	case *format.H264:
		return unitRemuxerH264()

	case *format.MPEG4Video:
		return unitRemuxerMPEG4Video

	default:
		return unitRemuxer(func(_ format.Format, _ int64, payload unit.Payload) unit.Payload {
			return payload
		})
	}
}
