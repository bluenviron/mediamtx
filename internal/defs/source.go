package defs

import (
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// Source is an entity that can provide a stream.
// it can be:
// - publisher
// - staticsources.Handler
// - redirectSource
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

// MediasInfo returns a description of medias.
func MediasInfo(medias []*description.Media) string {
	var formats []format.Format
	for _, media := range medias {
		formats = append(formats, media.Formats...)
	}

	return FormatsInfo(formats)
}
