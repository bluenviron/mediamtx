package stream

import (
	"bytes"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	mch264 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	mch265 "github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type formatUpdater func(format.Format, unit.Payload)

func formatUpdaterH265(forma format.Format, payload unit.Payload) {
	formatH265 := forma.(*format.H265)
	au := payload.(unit.PayloadH265)

	vps, sps, pps := formatH265.VPS, formatH265.SPS, formatH265.PPS
	update := false

	for _, nalu := range au {
		typ := mch265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case mch265.NALUType_VPS_NUT:
			if !bytes.Equal(nalu, formatH265.VPS) {
				vps = nalu
				update = true
			}

		case mch265.NALUType_SPS_NUT:
			if !bytes.Equal(nalu, formatH265.SPS) {
				sps = nalu
				update = true
			}

		case mch265.NALUType_PPS_NUT:
			if !bytes.Equal(nalu, formatH265.PPS) {
				pps = nalu
				update = true
			}
		}
	}

	if update {
		formatH265.SafeSetParams(vps, sps, pps)
	}
}

func formatUpdaterH264(forma format.Format, payload unit.Payload) {
	formatH264 := forma.(*format.H264)
	au := payload.(unit.PayloadH264)

	sps, pps := formatH264.SPS, formatH264.PPS
	update := false

	for _, nalu := range au {
		typ := mch264.NALUType(nalu[0] & 0x1F)

		switch typ {
		case mch264.NALUTypeSPS:
			if !bytes.Equal(nalu, sps) {
				sps = nalu
				update = true
			}

		case mch264.NALUTypePPS:
			if !bytes.Equal(nalu, pps) {
				pps = nalu
				update = true
			}
		}
	}

	if update {
		formatH264.SafeSetParams(sps, pps)
	}
}

func formatUpdaterMPEG4Video(forma format.Format, payload unit.Payload) {
	formatMPEG4Video := forma.(*format.MPEG4Video)
	frame := payload.(unit.PayloadMPEG4Video)

	if bytes.HasPrefix(frame, []byte{0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode)}) {
		end := bytes.Index(frame[4:], []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})
		if end < 0 {
			return
		}
		conf := frame[:end+4]

		if !bytes.Equal(conf, formatMPEG4Video.Config) {
			formatMPEG4Video.SafeSetParams(conf)
		}
	}
}

func newFormatUpdater(forma format.Format) formatUpdater {
	switch forma.(type) {
	case *format.H265:
		return formatUpdaterH265

	case *format.H264:
		return formatUpdaterH264

	case *format.MPEG4Video:
		return formatUpdaterMPEG4Video

	default:
		return formatUpdater(func(_ format.Format, _ unit.Payload) {
		})
	}
}
