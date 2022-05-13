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

	MessageTypeUserControl MessageType = 4

	MessageTypeDataAMF3    MessageType = 15
	MessageTypeCommandAMF3 MessageType = 17
	MessageTypeDataAMF0    MessageType = 18
	MessageTypeCommandAMF0 MessageType = 20

	MessageTypeAudio MessageType = 8
	MessageTypeVideo MessageType = 9
)
