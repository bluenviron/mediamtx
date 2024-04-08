package playback

import "github.com/bluenviron/mediacommon/pkg/formats/fmp4"

type muxer interface {
	writeInit(init *fmp4.Init)
	setTrack(trackID int)
	writeSample(dts int64, ptsOffset int32, isNonSyncSample bool, payload []byte) error
	writeFinalDTS(dts int64)
	flush() error
}
