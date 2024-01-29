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
	ID            uuid.UUID       `json:"id"`
	Created       time.Time       `json:"created"`
	RemoteAddr    string          `json:"remoteAddr"`
	State         APISRTConnState `json:"state"`
	Path          string          `json:"path"`
	Query         string          `json:"query"`
	BytesReceived uint64          `json:"bytesReceived"`
	BytesSent     uint64          `json:"bytesSent"`

	APISRTConnMetrics
}

// APISRTConnMetrics contains all the extra metrics for the SRT connections
// The metric names/comments are pulled from GoSRT
type APISRTConnMetrics struct {
	PktSent          uint64 `json:"pktSent"`        // The total number of sent DATA packets, including retransmitted packets
	PktRecv          uint64 `json:"pktRecv"`        // The total number of received DATA packets, including retransmitted packets
	PktSentUnique    uint64 `json:"pktSentUnique"`  // The total number of unique DATA packets sent by the SRT sender
	PktRecvUnique    uint64 `json:"pktRecvUnique"`  // The total number of unique original, retransmitted or recovered by the packet filter DATA packets received in time, decrypted without errors and, as a result, scheduled for delivery to the upstream application by the SRT receiver.
	PktSendLoss      uint64 `json:"pktSendLoss"`    // The total number of data packets considered or reported as lost at the sender side. Does not correspond to the packets detected as lost at the receiver side.
	PktRecvLoss      uint64 `json:"pktRecvLoss"`    // The total number of SRT DATA packets detected as presently missing (either reordered or lost) at the receiver side
	PktRetrans       uint64 `json:"pktRetrans"`     // The total number of retransmitted packets sent by the SRT sender
	PktRecvRetrans   uint64 `json:"pktRecvRetrans"` // The total number of retransmitted packets registered at the receiver side
	PktSentACK       uint64 `json:"pktSentACK"`     // The total number of sent ACK (Acknowledgement) control packets
	PktRecvACK       uint64 `json:"pktRecvACK"`     // The total number of received ACK (Acknowledgement) control packets
	PktSentNAK       uint64 `json:"pktSentNAK"`     // The total number of sent NAK (Negative Acknowledgement) control packets
	PktRecvNAK       uint64 `json:"pktRecvNAK"`     // The total number of received NAK (Negative Acknowledgement) control packets
	PktSentKM        uint64 `json:"pktSentKM"`      // The total number of sent KM (Key Material) control packets
	PktRecvKM        uint64 `json:"pktRecvKM"`      // The total number of received KM (Key Material) control packets
	UsSndDuration    uint64 `json:"usSndDuration"`  // The total accumulated time in microseconds, during which the SRT sender has some data to transmit, including packets that have been sent, but not yet acknowledged
	PktRecvBelated   uint64 `json:"pktRecvBelated"`
	PktSendDrop      uint64 `json:"pktSendDrop"`      // The total number of dropped by the SRT sender DATA packets that have no chance to be delivered in time
	PktRecvDrop      uint64 `json:"pktRecvDrop"`      // The total number of dropped by the SRT receiver and, as a result, not delivered to the upstream application DATA packets
	PktRecvUndecrypt uint64 `json:"pktRecvUndecrypt"` // The total number of packets that failed to be decrypted at the receiver side

	ByteSent          uint64 `json:"byteSent"`        // Same as pktSent, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecv          uint64 `json:"byteRecv"`        // Same as pktRecv, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteSentUnique    uint64 `json:"byteSentUnique"`  // Same as pktSentUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvUnique    uint64 `json:"byteRecvUnique"`  // Same as pktRecvUnique, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvLoss      uint64 `json:"byteRecvLoss"`    // Same as pktRecvLoss, but expressed in bytes, including payload and all the headers (IP, TCP, SRT), bytes for the presently missing (either reordered or lost) packets' payloads are estimated based on the average packet size
	ByteRetrans       uint64 `json:"byteRetrans"`     // Same as pktRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvRetrans   uint64 `json:"byteRecvRetrans"` // Same as pktRecvRetrans, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvBelated   uint64 `json:"byteRecvBelated"`
	ByteSendDrop      uint64 `json:"byteSendDrop"`      // Same as pktSendDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvDrop      uint64 `json:"byteRecvDrop"`      // Same as pktRecvDrop, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)
	ByteRecvUndecrypt uint64 `json:"byteRecvUndecrypt"` // Same as pktRecvUndecrypt, but expressed in bytes, including payload and all the headers (IP, TCP, SRT)

	UsPktSendPeriod       float64 `json:"usPktSendPeriod"`       // Current minimum time interval between which consecutive packets are sent, in microseconds
	PktFlowWindow         uint64  `json:"pktFlowWindow"`         // The maximum number of packets that can be "in flight"
	PktFlightSize         uint64  `json:"pktFlightSize"`         // The number of packets in flight
	MsRTT                 float64 `json:"msRTT"`                 // Smoothed round-trip time (SRTT), an exponentially-weighted moving average (EWMA) of an endpoint's RTT samples, in milliseconds
	MbpsSentRate          float64 `json:"mbpsSentRate"`          // Current transmission bandwidth, in Mbps
	MbpsRecvRate          float64 `json:"mbpsRecvRate"`          // Current receiving bandwidth, in Mbps
	MbpsLinkCapacity      float64 `json:"mbpsLinkCapacity"`      // Estimated capacity of the network link, in Mbps
	ByteAvailSendBuf      uint64  `json:"byteAvailSendBuf"`      // The available space in the sender's buffer, in bytes
	ByteAvailRecvBuf      uint64  `json:"byteAvailRecvBuf"`      // The available space in the receiver's buffer, in bytes
	MbpsMaxBW             float64 `json:"mbpsMaxBW"`             // Transmission bandwidth limit, in Mbps
	ByteMSS               uint64  `json:"byteMSS"`               // Maximum Segment Size (MSS), in bytes
	PktSendBuf            uint64  `json:"pktSendBuf"`            // The number of packets in the sender's buffer that are already scheduled for sending or even possibly sent, but not yet acknowledged
	ByteSendBuf           uint64  `json:"byteSendBuf"`           // Instantaneous (current) value of pktSndBuf, but expressed in bytes, including payload and all headers (IP, TCP, SRT)
	MsSendBuf             uint64  `json:"msSendBuf"`             // The timespan (msec) of packets in the sender's buffer (unacknowledged packets)
	MsSendTsbPdDelay      uint64  `json:"msSendTsbPdDelay"`      // Timestamp-based Packet Delivery Delay value of the peer
	PktRecvBuf            uint64  `json:"pktRecvBuf"`            // The number of acknowledged packets in receiver's buffer
	ByteRecvBuf           uint64  `json:"byteRecvBuf"`           // Instantaneous (current) value of pktRcvBuf, expressed in bytes, including payload and all headers (IP, TCP, SRT)
	MsRecvBuf             uint64  `json:"msRecvBuf"`             // The timespan (msec) of acknowledged packets in the receiver's buffer
	MsRecvTsbPdDelay      uint64  `json:"msRecvTsbPdDelay"`      // Timestamp-based Packet Delivery Delay value set on the socket via SRTO_RCVLATENCY or SRTO_LATENCY
	PktReorderTolerance   uint64  `json:"pktReorderTolerance"`   // Instant value of the packet reorder tolerance
	PktRecvAvgBelatedTime uint64  `json:"pktRecvAvgBelatedTime"` // Accumulated difference between the current time and the time-to-play of a packet that is received late
	PktSendLossRate       float64 `json:"pktSendLossRate"`       // Percentage of resent data vs. sent data
	PktRecvLossRate       float64 `json:"pktRecvLossRate"`       // Percentage of retransmitted data vs. received data
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
