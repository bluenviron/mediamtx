package base

// RawMessage is a message.
type RawMessage struct {
	ChunkStreamID   byte
	Timestamp       uint32
	Type            MessageType
	MessageStreamID uint32
	Body            []byte
}
