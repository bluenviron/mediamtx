package stats

func ptrInt64() *int64 {
	v := int64(0)
	return &v
}

// Stats contains statistics.
type Stats struct {
	// use pointers to avoid a crash on 32bit platforms
	// https://github.com/golang/go/issues/9959
	CountClients            *int64
	CountPublishers         *int64
	CountReaders            *int64
	CountSourcesRtsp        *int64
	CountSourcesRtspRunning *int64
	CountSourcesRtmp        *int64
	CountSourcesRtmpRunning *int64
}

// New allocates a Stats.
func New() *Stats {
	return &Stats{
		CountClients:            ptrInt64(),
		CountPublishers:         ptrInt64(),
		CountReaders:            ptrInt64(),
		CountSourcesRtsp:        ptrInt64(),
		CountSourcesRtspRunning: ptrInt64(),
		CountSourcesRtmp:        ptrInt64(),
		CountSourcesRtmpRunning: ptrInt64(),
	}
}

// Close closes a stats.
func (s *Stats) Close() {
}
