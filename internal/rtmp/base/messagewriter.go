package base

import (
	"io"
)

// MessageWriter is a message writer.
type MessageWriter struct {
	w                                   io.Writer
	chunkMaxBodyLen                     int
	lastMessageStreamIDPerChunkStreamID map[byte]uint32
}

// NewMessageWriter instantiates a MessageWriter.
func NewMessageWriter(w io.Writer) *MessageWriter {
	return &MessageWriter{
		w:                                   w,
		chunkMaxBodyLen:                     128,
		lastMessageStreamIDPerChunkStreamID: make(map[byte]uint32),
	}
}

// SetChunkSize sets the chunk size.
func (mw *MessageWriter) SetChunkSize(v int) {
	mw.chunkMaxBodyLen = v
}

// Write writes a Message.
func (mw *MessageWriter) Write(msg *Message) error {
	bodyLen := len(msg.Body)
	pos := 0
	first := true

	for {
		chunkBodyLen := bodyLen - pos
		if chunkBodyLen > mw.chunkMaxBodyLen {
			chunkBodyLen = mw.chunkMaxBodyLen
		}

		if first {
			first = false

			if v, ok := mw.lastMessageStreamIDPerChunkStreamID[msg.ChunkStreamID]; !ok || v != msg.MessageStreamID {
				err := Chunk0{
					ChunkStreamID:   msg.ChunkStreamID,
					Type:            msg.Type,
					MessageStreamID: msg.MessageStreamID,
					BodyLen:         uint32(bodyLen),
					Body:            msg.Body[pos : pos+chunkBodyLen],
				}.Write(mw.w)
				if err != nil {
					return err
				}

				mw.lastMessageStreamIDPerChunkStreamID[msg.ChunkStreamID] = msg.MessageStreamID
			} else {
				err := Chunk1{
					ChunkStreamID: msg.ChunkStreamID,
					Type:          msg.Type,
					BodyLen:       uint32(bodyLen),
					Body:          msg.Body[pos : pos+chunkBodyLen],
				}.Write(mw.w)
				if err != nil {
					return err
				}
			}
		} else {
			err := Chunk3{
				ChunkStreamID: msg.ChunkStreamID,
				Body:          msg.Body[pos : pos+chunkBodyLen],
			}.Write(mw.w)
			if err != nil {
				return err
			}
		}

		pos += chunkBodyLen

		if (bodyLen - pos) == 0 {
			return nil
		}
	}
}
