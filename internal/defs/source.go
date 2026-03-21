package defs

import (
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/bluenviron/mediamtx/internal/formatlabel"
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

// FormatsInfo returns a description of formats.
func FormatsInfo(formats []format.Format) string {
	labels := formatlabel.FormatsToLabels(formats)
	codecNames := make([]string, len(labels))
	for i, label := range labels {
		codecNames[i] = string(label)
	}

	return fmt.Sprintf("%d %s (%s)",
		len(formats),
		func() string {
			if len(formats) == 1 {
				return "track"
			}
			return "tracks"
		}(),
		strings.Join(codecNames, ", "))
}

// MediasInfo returns a description of medias.
func MediasInfo(medias []*description.Media) string {
	return FormatsInfo(gatherFormats(medias))
}
