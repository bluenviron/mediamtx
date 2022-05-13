package base

// Message is a message.
type Message struct {
	ChunkStreamID   byte
	Timestamp       uint32
	Type            MessageType
	MessageStreamID uint32
	Body            []byte
}
