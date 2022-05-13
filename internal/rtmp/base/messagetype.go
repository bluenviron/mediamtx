package base

// MessageType is a message type.
type MessageType byte

// message types.
const (
	MessageTypeSetChunkSize     MessageType = 1
	MessageTypeAbortMessage     MessageType = 2
	MessageTypeAcknowledge      MessageType = 3
	MessageTypeSetWindowAckSize MessageType = 5
	MessageTypeSetPeerBandwidth MessageType = 6
)
