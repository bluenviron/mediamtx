package core

func ptrInt64() *int64 {
	v := int64(0)
	return &v
}

type stats struct {
	// use pointers to avoid a crash on 32bit platforms
	// https://github.com/golang/go/issues/9959
	CountPublishers         *int64
	CountReaders            *int64
	CountSourcesRTSP        *int64
	CountSourcesRTSPRunning *int64
	CountSourcesRTMP        *int64
	CountSourcesRTMPRunning *int64
}

func newStats() *stats {
	return &stats{
		CountPublishers:         ptrInt64(),
		CountReaders:            ptrInt64(),
		CountSourcesRTSP:        ptrInt64(),
		CountSourcesRTSPRunning: ptrInt64(),
		CountSourcesRTMP:        ptrInt64(),
		CountSourcesRTMPRunning: ptrInt64(),
	}
}

func (s *stats) close() {
}
