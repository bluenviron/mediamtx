package core

import (
	"fmt"
	"strings"

	"github.com/aler9/gortsplib"
)

// source is an entity that can provide a stream.
// it can be:
// - a publisher
// - sourceStatic
// - sourceRedirect
type source interface {
	apiSourceDescribe() interface{}
}

func sourceTrackNames(tracks gortsplib.Tracks) []string {
	ret := make([]string, len(tracks))
	for i, t := range tracks {
		ret[i] = t.String()
	}
	return ret
}

func sourceTrackInfo(tracks gortsplib.Tracks) string {
	return fmt.Sprintf("%d %s (%s)",
		len(tracks),
		func() string {
			if len(tracks) == 1 {
				return "track"
			}
			return "tracks"
		}(),
		strings.Join(sourceTrackNames(tracks), ", "))
}
