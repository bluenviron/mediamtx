package webrtc

// Stats are WebRTC statistics.
type Stats struct {
	BytesReceived      uint64
	BytesSent          uint64
	RTPPacketsReceived uint64
	RTPPacketsSent     uint64
	RTPPacketsLost     uint64
	RTPPacketsJitter   float64
	// outbound RTP packets reported as lost by the remote peer through RTCP receiver reports.
	RTPPacketsReportedLost uint64
	// jitter of outbound RTP packets reported by the remote peer through RTCP receiver reports.
	RTPPacketsReportedJitter float64
	RTCPPacketsReceived      uint64
	RTCPPacketsSent          uint64
}
