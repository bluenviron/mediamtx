package webrtc

// Stats are WebRTC statistics.
type Stats struct {
	BytesReceived       uint64
	BytesSent           uint64
	RTPPacketsReceived  uint64
	RTPPacketsSent      uint64
	RTPPacketsLost      uint64
	RTPPacketsJitter    float64
	RTCPPacketsReceived uint64
	RTCPPacketsSent     uint64
}
