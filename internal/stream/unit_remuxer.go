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

// h265Remuxer holds back H265 access units that carry no picture data (e.g.
// a standalone prefix-SEI access unit) so that they can be prepended to the
// next access unit that shares their PTS and does contain a picture.
type h265Remuxer struct {
	pending    [][]byte
	pendingPTS int64
}

func (r *h265Remuxer) remux(forma format.Format, pts int64, payload unit.Payload) unit.Payload {
	formatH265 := forma.(*format.H265)
	au := payload.(unit.PayloadH265)

	hasVCL := false
	for _, nalu := range au {
		switch h265.NALUType((nalu[0] >> 1) & 0b111111) {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT,
			h265.NALUType_AUD_NUT, h265.NALUType_PREFIX_SEI_NUT, h265.NALUType_SUFFIX_SEI_NUT:
		default:
			hasVCL = true
		}
	}

	if !hasVCL {
		kept := make([][]byte, 0, len(au))
		for _, nalu := range au {
			typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

			switch typ {
			case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT,
				h265.NALUType_AUD_NUT, h265.NALUType_SUFFIX_SEI_NUT:
				continue
			}

			kept = append(kept, nalu)
		}
		if len(kept) != 0 {
			r.pending, r.pendingPTS = kept, pts
		}
		return unit.PayloadH265(nil)
	}

	var prefix [][]byte
	if r.pending != nil && r.pendingPTS == pts {
		prefix = r.pending
	}
	r.pending = nil

	isKeyFrame := false
	n := len(prefix)

	for _, nalu := range au {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT:
			continue

		case h265.NALUType_AUD_NUT:
			continue

		case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
			if !isKeyFrame {
				isKeyFrame = true

				// prepend parameters
				if formatH265.VPS != nil && formatH265.SPS != nil && formatH265.PPS != nil {
					n += 3
				}
			}
		}
		n++
	}

	filteredAU := make([][]byte, n)
	i := 0

	if isKeyFrame && formatH265.VPS != nil && formatH265.SPS != nil && formatH265.PPS != nil {
		filteredAU[0] = formatH265.VPS
		filteredAU[1] = formatH265.SPS
		filteredAU[2] = formatH265.PPS
		i = 3
	}

	i += copy(filteredAU[i:], prefix)

	for _, nalu := range au {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT:
			continue

		case h265.NALUType_AUD_NUT:
			continue
		}

		filteredAU[i] = nalu
		i++
	}

	return unit.PayloadH265(filteredAU)
}

// h264Remuxer holds back H264 access units that carry no VCL NALU (e.g. the
// standalone SEI access unit some DJI drones send ahead of every picture)
// so that they can be prepended to the next access unit that shares their
// PTS and does contain a picture.
type h264Remuxer struct {
	pending    [][]byte
	pendingPTS int64
}

func (r *h264Remuxer) remux(forma format.Format, pts int64, payload unit.Payload) unit.Payload {
	formatH264 := forma.(*format.H264)
	au := payload.(unit.PayloadH264)

	hasVCL := false
	for _, nalu := range au {
		switch h264.NALUType(nalu[0] & 0x1F) {
		case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter, h264.NALUTypeSEI:
		default:
			hasVCL = true
		}
	}

	if !hasVCL {
		kept := make([][]byte, 0, len(au))
		for _, nalu := range au {
			typ := h264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
				continue
			}

			kept = append(kept, nalu)
		}
		if len(kept) != 0 {
			r.pending, r.pendingPTS = kept, pts
		}
		return unit.PayloadH264(nil)
	}

	var prefix [][]byte
	if r.pending != nil && r.pendingPTS == pts {
		prefix = r.pending
	}
	r.pending = nil

	isKeyFrame := false
	n := len(prefix)

	for _, nalu := range au {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			continue

		case h264.NALUTypeIDR:
			if !isKeyFrame {
				isKeyFrame = true

				// prepend parameters
				if formatH264.SPS != nil && formatH264.PPS != nil {
					n += 2
				}
			}
		}
		n++
	}

	filteredAU := make([][]byte, n)
	i := 0

	if isKeyFrame && formatH264.SPS != nil && formatH264.PPS != nil {
		filteredAU[0] = formatH264.SPS
		filteredAU[1] = formatH264.PPS
		i = 2
	}

	i += copy(filteredAU[i:], prefix)

	for _, nalu := range au {
		typ := h264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
			continue

		case h264.NALUTypeAccessUnitDelimiter:
			continue
		}

		filteredAU[i] = nalu
		i++
	}

	return unit.PayloadH264(filteredAU)
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
		return (&h265Remuxer{}).remux

	case *format.H264:
		return (&h264Remuxer{}).remux

	case *format.MPEG4Video:
		return unitRemuxerMPEG4Video

	default:
		return unitRemuxer(func(_ format.Format, _ int64, payload unit.Payload) unit.Payload {
			return payload
		})
	}
}
