package stream

import (
	"bytes"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	mch264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	mch265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type unitRemuxer func(format.Format, unit.Payload) unit.Payload

func unitRemuxerH265(forma format.Format, payload unit.Payload) unit.Payload {
	formatH265 := forma.(*format.H265)
	au := payload.(unit.PayloadH265)

	isKeyFrame := false
	n := 0

	for _, nalu := range au {
		typ := mch265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case mch265.NALUType_VPS_NUT, mch265.NALUType_SPS_NUT, mch265.NALUType_PPS_NUT:
			continue

		case mch265.NALUType_AUD_NUT:
			continue

		case mch265.NALUType_IDR_W_RADL, mch265.NALUType_IDR_N_LP, mch265.NALUType_CRA_NUT:
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

	if n == 0 {
		return unit.PayloadH265(nil)
	}

	filteredAU := make([][]byte, n)
	i := 0

	if isKeyFrame && formatH265.VPS != nil && formatH265.SPS != nil && formatH265.PPS != nil {
		filteredAU[0] = formatH265.VPS
		filteredAU[1] = formatH265.SPS
		filteredAU[2] = formatH265.PPS
		i = 3
	}

	for _, nalu := range au {
		typ := mch265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case mch265.NALUType_VPS_NUT, mch265.NALUType_SPS_NUT, mch265.NALUType_PPS_NUT:
			continue

		case mch265.NALUType_AUD_NUT:
			continue
		}

		filteredAU[i] = nalu
		i++
	}

	return unit.PayloadH265(filteredAU)
}

func unitRemuxerH264(forma format.Format, payload unit.Payload) unit.Payload {
	formatH264 := forma.(*format.H264)
	au := payload.(unit.PayloadH264)

	isKeyFrame := false
	n := 0

	for _, nalu := range au {
		typ := mch264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case mch264.NALUTypeSPS, mch264.NALUTypePPS:
			continue

		case mch264.NALUTypeAccessUnitDelimiter:
			continue

		case mch264.NALUTypeIDR:
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

	if n == 0 {
		return unit.PayloadH264(nil)
	}

	filteredAU := make([][]byte, n)
	i := 0

	if isKeyFrame && formatH264.SPS != nil && formatH264.PPS != nil {
		filteredAU[0] = formatH264.SPS
		filteredAU[1] = formatH264.PPS
		i = 2
	}

	for _, nalu := range au {
		typ := mch264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case mch264.NALUTypeSPS, mch264.NALUTypePPS:
			continue

		case mch264.NALUTypeAccessUnitDelimiter:
			continue
		}

		filteredAU[i] = nalu
		i++
	}

	return unit.PayloadH264(filteredAU)
}

func unitRemuxerMPEG4Video(forma format.Format, payload unit.Payload) unit.Payload {
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
	case *format.H265:
		return unitRemuxerH265

	case *format.H264:
		return unitRemuxerH264

	case *format.MPEG4Video:
		return unitRemuxerMPEG4Video

	default:
		return unitRemuxer(func(_ format.Format, payload unit.Payload) unit.Payload {
			return payload
		})
	}
}
