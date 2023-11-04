package core

import (
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/description"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// source is an entity that can provide a stream.
// it can be:
// - publisher
// - staticSourceHandler
// - redirectSource
type source interface {
	logger.Writer
	APISourceDescribe() defs.APIPathSourceOrReader
}

func mediaDescription(media *description.Media) string {
	ret := make([]string, len(media.Formats))
	for i, forma := range media.Formats {
		ret[i] = forma.Codec()
	}
	return strings.Join(ret, "/")
}

func mediasDescription(medias []*description.Media) []string {
	ret := make([]string, len(medias))
	for i, media := range medias {
		ret[i] = mediaDescription(media)
	}
	return ret
}

func mediaInfo(medias []*description.Media) string {
	return fmt.Sprintf("%d %s (%s)",
		len(medias),
		func() string {
			if len(medias) == 1 {
				return "track"
			}
			return "tracks"
		}(),
		strings.Join(mediasDescription(medias), ", "))
}
