package rawmessage

import (
	"io"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
)

type writerChunkStream struct {
	mw                  *Writer
	lastMessageStreamID *uint32
	lastType            *chunk.MessageType
	lastBodyLen         *int
	lastTimestamp       *uint32
	lastTimestampDelta  *uint32
}

func (wc *writerChunkStream) write(msg *Message) error {
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
				err := chunk.Chunk0{
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
				err := chunk.Chunk1{
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
				err := chunk.Chunk2{
					ChunkStreamID:  msg.ChunkStreamID,
					TimestampDelta: *timestampDelta,
					Body:           msg.Body[pos : pos+chunkBodyLen],
				}.Write(wc.mw.w)
				if err != nil {
					return err
				}

			default:
				err := chunk.Chunk3{
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
			err := chunk.Chunk3{
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

// Writer is a raw message writer.
type Writer struct {
	w            io.Writer
	chunkSize    int
	chunkStreams map[byte]*writerChunkStream
}

// NewWriter allocates a Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:            w,
		chunkSize:    128,
		chunkStreams: make(map[byte]*writerChunkStream),
	}
}

// SetChunkSize sets the maximum chunk size.
func (w *Writer) SetChunkSize(v int) {
	w.chunkSize = v
}

// Write writes a Message.
func (w *Writer) Write(msg *Message) error {
	wc, ok := w.chunkStreams[msg.ChunkStreamID]
	if !ok {
		wc = &writerChunkStream{mw: w}
		w.chunkStreams[msg.ChunkStreamID] = wc
	}

	return wc.write(msg)
}
