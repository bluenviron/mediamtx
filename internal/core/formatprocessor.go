package core

import (
	"github.com/aler9/gortsplib/v2/pkg/format"
)

type formatProcessor interface {
	process(data, bool) error
}

func newFormatProcessor(forma format.Format, generateRTPPackets bool) (formatProcessor, error) {
	switch forma := forma.(type) {
	case *format.H264:
		return newFormatProcessorH264(forma, generateRTPPackets)

	case *format.H265:
		return newFormatProcessorH265(forma, generateRTPPackets)

	case *format.VP8:
		return newFormatProcessorVP8(forma, generateRTPPackets)

	case *format.VP9:
		return newFormatProcessorVP9(forma, generateRTPPackets)

	case *format.MPEG4Audio:
		return newFormatProcessorMPEG4Audio(forma, generateRTPPackets)

	case *format.Opus:
		return newFormatProcessorOpus(forma, generateRTPPackets)

	default:
		return newFormatProcessorGeneric(forma, generateRTPPackets)
	}
}
