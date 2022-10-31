package core

type streamTrackGeneric struct{}

func newStreamTrackGeneric() *streamTrackGeneric {
	return &streamTrackGeneric{}
}

func (t *streamTrackGeneric) process(dat data) []data {
	return []data{dat}
}
