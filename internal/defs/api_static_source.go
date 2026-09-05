package defs

import (
	"time"
)

// APIStaticSourceState is the state of a static source.
type APIStaticSourceState string

// static source states.
const (
	APIStaticSourceStateIdle    APIStaticSourceState = "idle"
	APIStaticSourceStateRunning APIStaticSourceState = "running"
	APIStaticSourceStateError   APIStaticSourceState = "error"
)

// APIStaticSourceTypeSpecific contains type-specific state.
type APIStaticSourceTypeSpecific interface {
	apiStaticSourceTypeSpecific()
}

// APIStaticSourceTypeSpecificRTMP contains type-specific state of RTMP static sources.
type APIStaticSourceTypeSpecificRTMP struct {
	RemoteAddr    string `json:"remoteAddr"`
	InboundBytes  uint64 `json:"inboundBytes"`
	OutboundBytes uint64 `json:"outboundBytes"`
}

func (*APIStaticSourceTypeSpecificRTMP) apiStaticSourceTypeSpecific() {}

// APIStaticSourceTypeSpecificRTSP contains type-specific state of RTSP static sources.
type APIStaticSourceTypeSpecificRTSP struct {
	RemoteAddr string `json:"remoteAddr"`
	Transport  string `json:"transport"`

	// inbound bytes
	InboundBytes uint64 `json:"inboundBytes"`
	// inbound RTP packets correctly received and processed
	InboundRTPPackets uint64 `json:"inboundRTPPackets"`
	// lost inbound RTP packets
	InboundRTPPacketsLost uint64 `json:"inboundRTPPacketsLost"`
	// inbound RTP packets that could not be processed
	InboundRTPPacketsInError uint64 `json:"inboundRTPPacketsInError"`
	// mean jitter of inbound RTP packets
	InboundRTPPacketsJitter float64 `json:"inboundRTPPacketsJitter"`
	// inbound RTCP packets correctly received and processed
	InboundRTCPPackets uint64 `json:"inboundRTCPPackets"`
	// inbound RTCP packets that could not be processed
	InboundRTCPPacketsInError uint64 `json:"inboundRTCPPacketsInError"`

	// outbound bytes
	OutboundBytes uint64 `json:"outboundBytes"`
	// outbound RTP packets
	OutboundRTPPackets uint64 `json:"outboundRTPPackets"`
	// outbound RTCP packets
	OutboundRTCPPackets uint64 `json:"outboundRTCPPackets"`
}

func (*APIStaticSourceTypeSpecificRTSP) apiStaticSourceTypeSpecific() {}

// APIStaticSourceTypeSpecificSRT contains type-specific state of SRT static sources.
type APIStaticSourceTypeSpecificSRT struct {
	RemoteAddr string `json:"remoteAddr"`

	// The metric names/comments are pulled from GoSRT

	// The total number of sent DATA packets, including retransmitted packets
	PacketsSent uint64 `json:"packetsSent"`
	// The total number of received DATA packets, including retransmitted packets
	PacketsReceived uint64 `json:"packetsReceived"`
	// The total number of unique DATA packets sent by the SRT sender
	PacketsSentUnique uint64 `json:"packetsSentUnique"`
	// The total number of unique original, retransmitted or recovered by the packet filter DATA packets
	// received in time, decrypted without errors and, as a result, scheduled for delivery to the
	// upstream application by the SRT receiver.
	PacketsReceivedUnique uint64 `json:"packetsReceivedUnique"`
	// The total number of data packets considered or reported as lost at the sender side.
	// Does not correspond to the packets detected as lost at the receiver side.
	PacketsSendLoss uint64 `json:"packetsSendLoss"`
	// The total number of SRT DATA packets detected as presently missing (either reordered or lost) at the receiver side
	PacketsReceivedLoss uint64 `json:"packetsReceivedLoss"`
	// The total number of retransmitted packets sent by the SRT sender
	PacketsRetrans uint64 `json:"packetsRetrans"`
	// The total number of retransmitted packets registered at the receiver side
	PacketsReceivedRetrans uint64 `json:"packetsReceivedRetrans"`
	// The total number of sent ACK (Acknowledgement) control packets
	PacketsSentACK uint64 `json:"packetsSentACK"`
	// The total number of received ACK (Acknowledgement) control packets
	PacketsReceivedACK uint64 `json:"packetsReceivedACK"`
	// The total number of sent NAK (Negative Acknowledgement) control packets
	PacketsSentNAK uint64 `json:"packetsSentNAK"`
	// The total number of received NAK (Negative Acknowledgement) control packets
	PacketsReceivedNAK uint64 `json:"packetsReceivedNAK"`
	// The total number of sent KM (Key Material) control packets
	PacketsSentKM uint64 `json:"packetsSentKM"`
	// The total number of received KM (Key Material) control packets
	PacketsReceivedKM uint64 `json:"packetsReceivedKM"`
	// The total accumulated time in microseconds, during which the SRT sender has some data to transmit,
	// including packets that have been sent, but not yet acknowledged
	UsSndDuration uint64 `json:"usSndDuration"`

	PacketsReceivedBelated uint64 `json:"packetsReceivedBelated"`
	// The total number of dropped by the SRT sender DATA packets that have no chance to be delivered in time
	PacketsSendDrop uint64 `json:"packetsSendDrop"`
	// The total number of dropped by the SRT receiver and, as a result,
	// not delivered to the upstream application DATA packets
	PacketsReceivedDrop uint64 `json:"packetsReceivedDrop"`
	// The total number of packets that failed to be decrypted at the receiver side
	PacketsReceivedUndecrypt uint64 `json:"packetsReceivedUndecrypt"`

	// Same as packetsReceived, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesReceived uint64 `json:"bytesReceived"`
	// Same as packetsSent, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesSent uint64 `json:"bytesSent"`
	// Same as packetsSentUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesSentUnique uint64 `json:"bytesSentUnique"`
	// Same as packetsReceivedUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesReceivedUnique uint64 `json:"bytesReceivedUnique"`
	// Same as packetsReceivedLoss, but expressed in bytes, including payload and all the headers (IP, TCP, SRT),
	// bytes for the presently missing (either reordered or lost) packets' payloads are estimated
	// based on the average packet size
	BytesReceivedLoss uint64 `json:"bytesReceivedLoss"`
	// Same as packetsRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesRetrans uint64 `json:"bytesRetrans"`
	// Same as packetsReceivedRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesReceivedRetrans uint64 `json:"bytesReceivedRetrans"`
	// Same as PacketsReceivedBelated, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesReceivedBelated uint64 `json:"bytesReceivedBelated"`
	// Same as packetsSendDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesSendDrop uint64 `json:"bytesSendDrop"`
	// Same as packetsReceivedDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesReceivedDrop uint64 `json:"bytesReceivedDrop"`
	// Same as packetsReceivedUndecrypt, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	BytesReceivedUndecrypt uint64 `json:"bytesReceivedUndecrypt"`

	// Current minimum time interval between which consecutive packets are sent, in microseconds
	UsPacketsSendPeriod float64 `json:"usPacketsSendPeriod"`
	// The maximum number of packets that can be "in flight"
	PacketsFlowWindow uint64 `json:"packetsFlowWindow"`
	// The number of packets in flight
	PacketsFlightSize uint64 `json:"packetsFlightSize"`
	// Smoothed round-trip time (SRTT), an exponentially-weighted moving average (EWMA)
	// of an endpoint's RTT samples, in milliseconds
	MsRTT float64 `json:"msRTT"`
	// Current transmission bandwidth, in Mbps
	MbpsSendRate float64 `json:"mbpsSendRate"`
	// Current receiving bandwidth, in Mbps
	MbpsReceiveRate float64 `json:"mbpsReceiveRate"`
	// Estimated capacity of the network link, in Mbps
	MbpsLinkCapacity float64 `json:"mbpsLinkCapacity"`
	// The available space in the sender's buffer, in bytes
	BytesAvailSendBuf uint64 `json:"bytesAvailSendBuf"`
	// The available space in the receiver's buffer, in bytes
	BytesAvailReceiveBuf uint64 `json:"bytesAvailReceiveBuf"`
	// Transmission bandwidth limit, in Mbps
	MbpsMaxBW float64 `json:"mbpsMaxBW"`
	// Maximum Segment Size (MSS), in bytes
	ByteMSS uint64 `json:"byteMSS"`
	// The number of packets in the sender's buffer that are already scheduled
	// for sending or even possibly sent, but not yet acknowledged
	PacketsSendBuf uint64 `json:"packetsSendBuf"`
	// Instantaneous (current) value of packetsSndBuf, but expressed in bytes,
	// including payload and all headers (IP, TCP, SRT)
	BytesSendBuf uint64 `json:"bytesSendBuf"`
	// The timespan (msec) of packets in the sender's buffer (unacknowledged packets)
	MsSendBuf uint64 `json:"msSendBuf"`
	// Timestamp-based Packet Delivery Delay value of the peer
	MsSendTsbPdDelay uint64 `json:"msSendTsbPdDelay"`
	// The number of acknowledged packets in receiver's buffer
	PacketsReceiveBuf uint64 `json:"packetsReceiveBuf"`
	// Instantaneous (current) value of packetsRcvBuf, expressed in bytes, including payload and all headers (IP, TCP, SRT)
	BytesReceiveBuf uint64 `json:"bytesReceiveBuf"`
	// The timespan (msec) of acknowledged packets in the receiver's buffer
	MsReceiveBuf uint64 `json:"msReceiveBuf"`
	// Timestamp-based Packet Delivery Delay value set on the socket via SRTO_RCVLATENCY or SRTO_LATENCY
	MsReceiveTsbPdDelay uint64 `json:"msReceiveTsbPdDelay"`
	// Instant value of the packet reorder tolerance
	PacketsReorderTolerance uint64 `json:"packetsReorderTolerance"`
	// Accumulated difference between the current time and the time-to-play of a packet that is received late
	PacketsReceivedAvgBelatedTime uint64 `json:"packetsReceivedAvgBelatedTime"`
	// Percentage of resent data vs. sent data
	PacketsSendLossRate float64 `json:"packetsSendLossRate"`
	// Percentage of retransmitted data vs. received data
	PacketsReceivedLossRate float64 `json:"packetsReceivedLossRate"`
}

func (*APIStaticSourceTypeSpecificSRT) apiStaticSourceTypeSpecific() {}

// APIStaticSourceTypeSpecificWebRTC contains type-specific state of WebRTC static sources.
type APIStaticSourceTypeSpecificWebRTC struct {
	RemoteAddr                string  `json:"remoteAddr"`
	PeerConnectionEstablished bool    `json:"peerConnectionEstablished"`
	LocalCandidate            string  `json:"localCandidate"`
	RemoteCandidate           string  `json:"remoteCandidate"`
	InboundBytes              uint64  `json:"inboundBytes"`
	InboundRTPPackets         uint64  `json:"inboundRTPPackets"`
	InboundRTPPacketsLost     uint64  `json:"inboundRTPPacketsLost"`
	InboundRTPPacketsJitter   float64 `json:"inboundRTPPacketsJitter"`
	InboundRTCPPackets        uint64  `json:"inboundRTCPPackets"`
	OutboundBytes             uint64  `json:"outboundBytes"`
	OutboundRTPPackets        uint64  `json:"outboundRTPPackets"`
	OutboundRTCPPackets       uint64  `json:"outboundRTCPPackets"`
}

func (*APIStaticSourceTypeSpecificWebRTC) apiStaticSourceTypeSpecific() {}

// APIStaticSourceTypeSpecificMoQ contains type-specific state of MoQ static sources.
type APIStaticSourceTypeSpecificMoQ struct {
	RemoteAddr   string `json:"remoteAddr"`
	Transport    string `json:"transport"`
	InboundBytes uint64 `json:"inboundBytes"`
}

func (*APIStaticSourceTypeSpecificMoQ) apiStaticSourceTypeSpecific() {}

// APIStaticSource is a static source.
type APIStaticSource struct {
	Created      time.Time                   `json:"created"`
	State        APIStaticSourceState        `json:"state"`
	LastError    string                      `json:"lastError"`
	Type         APIPathSourceType           `json:"type"`
	TypeSpecific APIStaticSourceTypeSpecific `json:"typeSpecific"`
}
