package defs

// SourceStats is a marker interface for all stats types.
// It allows multiple concrete stats implementations (RTSP, SRT, etc.).
type SourceStats interface {
	isSourceStats()
}

// SourceStatsProvider is an OPTIONAL capability.
// Only sources that can provide stats will implement this.
type SourceStatsProvider interface {
	SourceStats() SourceStats
}

// RTSPSourceStats - RTSP-specific stats (API-safe, no gortsplib leakage)
type RTSPSourceStats struct {
	PacketsReceived *uint64  `json:"inboundRTPPackets"`
	PacketsLost     *uint64  `json:"inboundRTPPacketsLost"`
	PacketsInError  *uint64  `json:"inboundRTPPacketsInError"`
	Jitter          *float64 `json:"inboundRTPPacketsJitter"`
}

func (*RTSPSourceStats) isSourceStats() {}
