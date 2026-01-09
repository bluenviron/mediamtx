package defs

import (
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/codecprocessor"
)

// Source is an entity that can provide a stream.
// it can be:
// - Publisher
// - staticsources.Handler
// - core.sourceRedirect
type Source interface {
	logger.Writer
	APISourceDescribe() APIPathSourceOrReader
}

// FormatsToCodecs returns the name of codecs of given formats.
func FormatsToCodecs(formats []format.Format) []string {
	ret := make([]string, len(formats))
	for i, forma := range formats {
		ret[i] = forma.Codec()
	}
	return ret
}

// FormatsInfo returns a description of formats.
func FormatsInfo(formats []format.Format) string {
	return fmt.Sprintf("%d %s (%s)",
		len(formats),
		func() string {
			if len(formats) == 1 {
				return "track"
			}
			return "tracks"
		}(),
		strings.Join(FormatsToCodecs(formats), ", "))
}

// MediasToCodecs returns the name of codecs of given formats.
func MediasToCodecs(medias []*description.Media) []string {
	var formats []format.Format
	for _, media := range medias {
		formats = append(formats, media.Formats...)
	}

	return FormatsToCodecs(formats)
}

// MediasToResolutions returns the resolutions of given medias.
func MediasToResolutions(medias []*description.Media) []string {
	var ret []string
	for _, media := range medias {
		for _, forma := range media.Formats {
			ret = append(ret, getResolution(forma))
		}
	}
	return ret
}

func getResolution(forma format.Format) string {
	if h264f, ok := forma.(*format.H264); ok {
		width, height := codecprocessor.ExtractH264Resolution(h264f.SPS)
		if width > 0 && width <= 10000 && height > 0 && height <= 10000 {
			return fmt.Sprintf("%dx%d", width, height)
		}
	} else if h265f, ok := forma.(*format.H265); ok {
		width, height := codecprocessor.ExtractH265Resolution(h265f.SPS)
		if width > 0 && width <= 10000 && height > 0 && height <= 10000 {
			return fmt.Sprintf("%dx%d", width, height)
		}
	} else if mpeg4f, ok := forma.(*format.MPEG4Video); ok {
		width, height := codecprocessor.ExtractMPEG4Resolution(mpeg4f.Config)
		if width > 0 && width <= 10000 && height > 0 && height <= 10000 {
			return fmt.Sprintf("%dx%d", width, height)
		}
	} else if _, ok := forma.(*format.MPEG1Video); ok {
		width, height := codecprocessor.ExtractMPEG1Resolution(codecprocessor.MPEG1VideoDefaultConfig)
		if width > 0 && width <= 10000 && height > 0 && height <= 10000 {
			return fmt.Sprintf("%dx%d", width, height)
		}
	}
	return ""
}

// MediasInfo returns a description of medias.
func MediasInfo(medias []*description.Media) string {
	var formats []format.Format
	for _, media := range medias {
		formats = append(formats, media.Formats...)
	}

	return FormatsInfo(formats)
}
