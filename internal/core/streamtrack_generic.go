package core

import (
	"github.com/aler9/gortsplib"
)

type streamTrackGeneric struct {
	writeDataInner func(*data)
}

func newStreamTrackGeneric(track gortsplib.Track, writeDataInner func(*data)) *streamTrackGeneric {
	return &streamTrackGeneric{
		writeDataInner: writeDataInner,
	}
}

func (t *streamTrackGeneric) writeData(data *data) {
	t.writeDataInner(data)
}
