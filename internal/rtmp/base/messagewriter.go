package base

import (
	"io"
)

type messageWriterChunkStream struct {
	mw                  *MessageWriter
	lastMessageStreamID *uint32
	lastType            *MessageType
	lastBodyLen         *int
	lastTimestamp       *uint32
	lastTimestampDelta  *uint32
}

func (wc *messageWriterChunkStream) write(msg *Message) error {
	bodyLen := len(msg.Body)
	pos := 0
	firstChunk := true

	var timestampDelta *uint32
	if wc.lastTimestamp != nil {
		diff := int64(msg.Timestamp) - int64(*wc.lastTimestamp)

		// use delta only if it is positive
		if diff >= 0 {
			v := uint32(diff)
			timestampDelta = &v
		}
	}

	for {
		chunkBodyLen := bodyLen - pos
		if chunkBodyLen > wc.mw.chunkSize {
			chunkBodyLen = wc.mw.chunkSize
		}

		if firstChunk {
			firstChunk = false

			switch {
			case wc.lastMessageStreamID == nil || timestampDelta == nil || *wc.lastMessageStreamID != msg.MessageStreamID:
				err := Chunk0{
					ChunkStreamID:   msg.ChunkStreamID,
					Timestamp:       msg.Timestamp,
					Type:            msg.Type,
					MessageStreamID: msg.MessageStreamID,
					BodyLen:         uint32(bodyLen),
					Body:            msg.Body[pos : pos+chunkBodyLen],
				}.Write(wc.mw.w)
				if err != nil {
					return err
				}

			case *wc.lastType != msg.Type || *wc.lastBodyLen != bodyLen:
				err := Chunk1{
					ChunkStreamID:  msg.ChunkStreamID,
					TimestampDelta: *timestampDelta,
					Type:           msg.Type,
					BodyLen:        uint32(bodyLen),
					Body:           msg.Body[pos : pos+chunkBodyLen],
				}.Write(wc.mw.w)
				if err != nil {
					return err
				}

			case wc.lastTimestampDelta == nil || *wc.lastTimestampDelta != *timestampDelta:
				err := Chunk2{
					ChunkStreamID:  msg.ChunkStreamID,
					TimestampDelta: *timestampDelta,
					Body:           msg.Body[pos : pos+chunkBodyLen],
				}.Write(wc.mw.w)
				if err != nil {
					return err
				}

			default:
				err := Chunk3{
					ChunkStreamID: msg.ChunkStreamID,
					Body:          msg.Body[pos : pos+chunkBodyLen],
				}.Write(wc.mw.w)
				if err != nil {
					return err
				}
			}

			v1 := msg.MessageStreamID
			wc.lastMessageStreamID = &v1
			v2 := msg.Type
			wc.lastType = &v2
			v3 := bodyLen
			wc.lastBodyLen = &v3
			v4 := msg.Timestamp
			wc.lastTimestamp = &v4

			if timestampDelta != nil {
				v5 := *timestampDelta
				wc.lastTimestampDelta = &v5
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

// NewMessageWriter allocates a MessageWriter.
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
	wc, ok := mw.chunkStreams[msg.ChunkStreamID]
	if !ok {
		wc = &messageWriterChunkStream{mw: mw}
		mw.chunkStreams[msg.ChunkStreamID] = wc
	}

	return wc.write(msg)
}
