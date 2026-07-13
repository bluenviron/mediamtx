package defs

// StaticSourceStats is a marker interface for all static-source stats types.
// It allows multiple concrete stats implementations (RTSP, SRT, etc.).
// Dynamic sources are not covered by this interface.
type StaticSourceStats interface {
	isStaticSourceStats()
}

// StaticSourceStatsProvider is an OPTIONAL capability.
// Only static sources that can provide stats will implement this.
type StaticSourceStatsProvider interface {
	SourceStats() StaticSourceStats
}

// BaseSourceStats holds packet statistics common to all packet-based sources.
// Packet loss is always reported; Jitter is a pointer so that protocols that
// cannot provide it (e.g. SRT) leave it nil and it is omitted from the API.
type BaseSourceStats struct {
	PacketsReceived uint64   `json:"inboundRTPPackets"`
	PacketsLost     uint64   `json:"inboundRTPPacketsLost"`
	Jitter          *float64 `json:"inboundRTPPacketsJitter,omitempty"`
}

// RTSPSourceStats - RTSP-specific stats (API-safe, no gortsplib leakage)
type RTSPSourceStats struct {
	BaseSourceStats
	PacketsInError uint64 `json:"inboundRTPPacketsInError"`
}

func (*RTSPSourceStats) isStaticSourceStats() {}

// RTPSourceStats - raw RTP (udp+rtp) source stats.
// Jitter is not computed for raw RTP and is left nil.
type RTPSourceStats struct {
	BaseSourceStats
}

func (*RTPSourceStats) isStaticSourceStats() {}

// WebRTCSourceStats - WebRTC (WHEP) source stats.
type WebRTCSourceStats struct {
	BaseSourceStats
}

func (*WebRTCSourceStats) isStaticSourceStats() {}

// SRTSourceStats - SRT source stats.
// SRT exposes receive packet loss but no RTP-style jitter, so Jitter is nil.
type SRTSourceStats struct {
	BaseSourceStats
}

func (*SRTSourceStats) isStaticSourceStats() {}
