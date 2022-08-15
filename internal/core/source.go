package core

import (
	"fmt"
	"reflect"
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

func sourceTrackInfo(tracks gortsplib.Tracks) string {
	trackCodecs := make([]string, len(tracks))
	for i, t := range tracks {
		n := reflect.TypeOf(t).Elem().Name()
		n = n[len("Track"):]
		trackCodecs[i] = n
	}

	return fmt.Sprintf("%d %s (%s)",
		len(tracks),
		func() string {
			if len(tracks) == 1 {
				return "track"
			}
			return "tracks"
		}(),
		strings.Join(trackCodecs, ", "))
}
