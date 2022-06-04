package chunk

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

	MessageTypeCommandAMF3 MessageType = 17
	MessageTypeCommandAMF0 MessageType = 20

	MessageTypeDataAMF3 MessageType = 15
	MessageTypeDataAMF0 MessageType = 18

	MessageTypeAudio MessageType = 8
	MessageTypeVideo MessageType = 9
)
