package base

import (
	"io"
)

type messageWriterChunkStream struct {
	mw                  *MessageWriter
	lastMessageStreamID *uint32
}

func (wc *messageWriterChunkStream) write(msg *Message) error {
	bodyLen := len(msg.Body)
	pos := 0
	firstChunk := true

	for {
		chunkBodyLen := bodyLen - pos
		if chunkBodyLen > wc.mw.chunkSize {
			chunkBodyLen = wc.mw.chunkSize
		}

		if firstChunk {
			firstChunk = false

			if wc.lastMessageStreamID == nil || *wc.lastMessageStreamID != msg.MessageStreamID {
				err := Chunk0{
					ChunkStreamID:   msg.ChunkStreamID,
					Type:            msg.Type,
					MessageStreamID: msg.MessageStreamID,
					BodyLen:         uint32(bodyLen),
					Body:            msg.Body[pos : pos+chunkBodyLen],
				}.Write(wc.mw.w)
				if err != nil {
					return err
				}

				v := msg.MessageStreamID
				wc.lastMessageStreamID = &v
			} else {
				err := Chunk1{
					ChunkStreamID: msg.ChunkStreamID,
					Type:          msg.Type,
					BodyLen:       uint32(bodyLen),
					Body:          msg.Body[pos : pos+chunkBodyLen],
				}.Write(wc.mw.w)
				if err != nil {
					return err
				}
			}
		} else {
			err := Chunk3{
				ChunkStreamID: msg.ChunkStreamID,
				Body:          msg.Body[pos : pos+chunkBodyLen],
			}.Write(wc.mw.w)
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

// MessageWriter is a message writer.
type MessageWriter struct {
	w            io.Writer
	chunkSize    int
	chunkStreams map[byte]*messageWriterChunkStream
}

// NewMessageWriter instantiates a MessageWriter.
func NewMessageWriter(w io.Writer) *MessageWriter {
	return &MessageWriter{
		w:            w,
		chunkSize:    128,
		chunkStreams: make(map[byte]*messageWriterChunkStream),
	}
}

// SetChunkSize sets the maximum chunk size.
func (mw *MessageWriter) SetChunkSize(v int) {
	mw.chunkSize = v
}

// Write writes a Message.
func (mw *MessageWriter) Write(msg *Message) error {
	cs, ok := mw.chunkStreams[msg.ChunkStreamID]
	if !ok {
		cs = &messageWriterChunkStream{mw: mw}
		mw.chunkStreams[msg.ChunkStreamID] = cs
	}

	return cs.write(msg)
}
