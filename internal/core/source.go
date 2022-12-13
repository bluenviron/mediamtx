package core

import (
	"fmt"
	"strings"

	"github.com/aler9/gortsplib/v2/pkg/media"
)

// source is an entity that can provide a stream.
// it can be:
// - a publisher
// - sourceStatic
// - sourceRedirect
type source interface {
	apiSourceDescribe() interface{}
}

func mediaDescription(media *media.Media) string {
	ret := make([]string, len(media.Formats))
	for i, forma := range media.Formats {
		ret[i] = forma.String()
	}
	return strings.Join(ret, "/")
}

func mediasDescription(medias media.Medias) []string {
	ret := make([]string, len(medias))
	for i, media := range medias {
		ret[i] = mediaDescription(media)
	}
	return ret
}

func sourceMediaInfo(medias media.Medias) string {
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
