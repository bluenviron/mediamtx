package defs

import (
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
)

// Source is an entity that can provide a stream.
// it can be:
// - Publisher
// - staticsources.Handler
// - core.sourceRedirect
type Source interface {
	logger.Writer
	APISourceDescribe() *APIPathSource
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

func gatherFormats(medias []*description.Media) []format.Format {
	n := 0
	for _, media := range medias {
		n += len(media.Formats)
	}

	if n == 0 {
		return nil
	}

	formats := make([]format.Format, n)
	n = 0

	for _, media := range medias {
		n += copy(formats[n:], media.Formats)
	}

	return formats
}

// MediasToCodecs returns the name of codecs of given formats.
func MediasToCodecs(medias []*description.Media) []string {
	return FormatsToCodecs(gatherFormats(medias))
}

// MediasInfo returns a description of medias.
func MediasInfo(medias []*description.Media) string {
	return FormatsInfo(gatherFormats(medias))
}
