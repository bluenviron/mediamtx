package core

type streamTrackGeneric struct{}

func newStreamTrackGeneric() *streamTrackGeneric {
	return &streamTrackGeneric{}
}

func (t *streamTrackGeneric) onData(dat data, hasNonRTSPReaders bool) {
}
