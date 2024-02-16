package defs

import (
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
)

// APIError is a generic error.
type APIError struct {
	Error string `json:"error"`
}

// APIPathConfList is a list of path configurations.
type APIPathConfList struct {
	ItemCount int          `json:"itemCount"`
	PageCount int          `json:"pageCount"`
	Items     []*conf.Path `json:"items"`
}

// APIPathSourceOrReader is a source or a reader.
type APIPathSourceOrReader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// APIPath is a path.
type APIPath struct {
	Name          string                  `json:"name"`
	ConfName      string                  `json:"confName"`
	Source        *APIPathSourceOrReader  `json:"source"`
	Ready         bool                    `json:"ready"`
	ReadyTime     *time.Time              `json:"readyTime"`
	Tracks        []string                `json:"tracks"`
	BytesReceived uint64                  `json:"bytesReceived"`
	BytesSent     uint64                  `json:"bytesSent"`
	Readers       []APIPathSourceOrReader `json:"readers"`
}

// APIPathList is a list of paths.
type APIPathList struct {
	ItemCount int        `json:"itemCount"`
	PageCount int        `json:"pageCount"`
	Items     []*APIPath `json:"items"`
}

// APIHLSMuxer is an HLS muxer.
type APIHLSMuxer struct {
	Path        string    `json:"path"`
	Created     time.Time `json:"created"`
	LastRequest time.Time `json:"lastRequest"`
	BytesSent   uint64    `json:"bytesSent"`
}

// APIHLSMuxerList is a list of HLS muxers.
type APIHLSMuxerList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*APIHLSMuxer `json:"items"`
}

// APIRTMPConnState is the state of a RTMP connection.
type APIRTMPConnState string

// states.
const (
	APIRTMPConnStateIdle    APIRTMPConnState = "idle"
	APIRTMPConnStateRead    APIRTMPConnState = "read"
	APIRTMPConnStatePublish APIRTMPConnState = "publish"
)

// APIRTMPConn is a RTMP connection.
type APIRTMPConn struct {
	ID            uuid.UUID        `json:"id"`
	Created       time.Time        `json:"created"`
	RemoteAddr    string           `json:"remoteAddr"`
	State         APIRTMPConnState `json:"state"`
	Path          string           `json:"path"`
	Query         string           `json:"query"`
	BytesReceived uint64           `json:"bytesReceived"`
	BytesSent     uint64           `json:"bytesSent"`
}

// APIRTMPConnList is a list of RTMP connections.
type APIRTMPConnList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*APIRTMPConn `json:"items"`
}

// APIRTSPConn is a RTSP connection.
type APIRTSPConn struct {
	ID            uuid.UUID `json:"id"`
	Created       time.Time `json:"created"`
	RemoteAddr    string    `json:"remoteAddr"`
	BytesReceived uint64    `json:"bytesReceived"`
	BytesSent     uint64    `json:"bytesSent"`
}

// APIRTSPConnsList is a list of RTSP connections.
type APIRTSPConnsList struct {
	ItemCount int            `json:"itemCount"`
	PageCount int            `json:"pageCount"`
	Items     []*APIRTSPConn `json:"items"`
}

// APIRTSPSessionState is the state of a RTSP session.
type APIRTSPSessionState string

// states.
const (
	APIRTSPSessionStateIdle    APIRTSPSessionState = "idle"
	APIRTSPSessionStateRead    APIRTSPSessionState = "read"
	APIRTSPSessionStatePublish APIRTSPSessionState = "publish"
)

// APIRTSPSession is a RTSP session.
type APIRTSPSession struct {
	ID            uuid.UUID           `json:"id"`
	Created       time.Time           `json:"created"`
	RemoteAddr    string              `json:"remoteAddr"`
	State         APIRTSPSessionState `json:"state"`
	Path          string              `json:"path"`
	Query         string              `json:"query"`
	Transport     *string             `json:"transport"`
	BytesReceived uint64              `json:"bytesReceived"`
	BytesSent     uint64              `json:"bytesSent"`
}

// APIRTSPSessionList is a list of RTSP sessions.
type APIRTSPSessionList struct {
	ItemCount int               `json:"itemCount"`
	PageCount int               `json:"pageCount"`
	Items     []*APIRTSPSession `json:"items"`
}

// APISRTConnState is the state of a SRT connection.
type APISRTConnState string

// states.
const (
	APISRTConnStateIdle    APISRTConnState = "idle"
	APISRTConnStateRead    APISRTConnState = "read"
	APISRTConnStatePublish APISRTConnState = "publish"
)

// APISRTConn is a SRT connection.
type APISRTConn struct {
	ID         uuid.UUID       `json:"id"`
	Created    time.Time       `json:"created"`
	RemoteAddr string          `json:"remoteAddr"`
	State      APISRTConnState `json:"state"`
	Path       string          `json:"path"`
	Query      string          `json:"query"`

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
	// ??
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

// APISRTConnList is a list of SRT connections.
type APISRTConnList struct {
	ItemCount int           `json:"itemCount"`
	PageCount int           `json:"pageCount"`
	Items     []*APISRTConn `json:"items"`
}

// APIWebRTCSessionState is the state of a WebRTC connection.
type APIWebRTCSessionState string

// states.
const (
	APIWebRTCSessionStateRead    APIWebRTCSessionState = "read"
	APIWebRTCSessionStatePublish APIWebRTCSessionState = "publish"
)

// APIWebRTCSession is a WebRTC session.
type APIWebRTCSession struct {
	ID                        uuid.UUID             `json:"id"`
	Created                   time.Time             `json:"created"`
	RemoteAddr                string                `json:"remoteAddr"`
	PeerConnectionEstablished bool                  `json:"peerConnectionEstablished"`
	LocalCandidate            string                `json:"localCandidate"`
	RemoteCandidate           string                `json:"remoteCandidate"`
	State                     APIWebRTCSessionState `json:"state"`
	Path                      string                `json:"path"`
	Query                     string                `json:"query"`
	BytesReceived             uint64                `json:"bytesReceived"`
	BytesSent                 uint64                `json:"bytesSent"`
}

// APIWebRTCSessionList is a list of WebRTC sessions.
type APIWebRTCSessionList struct {
	ItemCount int                 `json:"itemCount"`
	PageCount int                 `json:"pageCount"`
	Items     []*APIWebRTCSession `json:"items"`
}

// APIRecordingSegment is a recording segment.
type APIRecordingSegment struct {
	Start time.Time `json:"start"`
}

// APIRecording is a recording.
type APIRecording struct {
	Name     string                 `json:"name"`
	Segments []*APIRecordingSegment `json:"segments"`
}

// APIRecordingList is a list of recordings.
type APIRecordingList struct {
	ItemCount int             `json:"itemCount"`
	PageCount int             `json:"pageCount"`
	Items     []*APIRecording `json:"items"`
}
