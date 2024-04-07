package playback

import "github.com/bluenviron/mediacommon/pkg/formats/fmp4"

type muxer interface {
	writeInit(init []byte)
	setTrack(trackID int)
	writeSample(normalizedElapsed int64, sample *fmp4.PartSample)
	flush() error
}
