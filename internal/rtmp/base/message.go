package base

// Message is a message.
type Message struct {
	ChunkStreamID   byte
	Timestamp       uint32
	Typ             byte
	MessageStreamID uint32
	Body            []byte
}
