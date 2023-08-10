package rawmessage

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/chunk"
)

type writerChunkStream struct {
	mw                  *Writer
	lastMessageStreamID *uint32
	lastType            *uint8
	lastBodyLen         *uint32
	lastTimestamp       *int64
	lastTimestampDelta  *int64
}

func (wc *writerChunkStream) writeChunk(c chunk.Chunk) error {
	// check if we received an acknowledge
	if wc.mw.checkAcknowledge && wc.mw.ackWindowSize != 0 {
		diff := uint32(wc.mw.bcw.Count()) - wc.mw.ackValue

		if diff > (wc.mw.ackWindowSize * 3 / 2) {
			return fmt.Errorf("no acknowledge received within window")
		}
	}

	buf, err := c.Marshal()
	if err != nil {
		return err
	}

	_, err = wc.mw.bw.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func (wc *writerChunkStream) writeMessage(msg *Message) error {
	bodyLen := uint32(len(msg.Body))
	pos := uint32(0)
	firstChunk := true

	// convert timestamp to milliseconds before splitting message in chunks
	/// otherwise timestampDelta gets messed up.
	timestamp := int64(msg.Timestamp / time.Millisecond)

	var timestampDelta *int64
	if wc.lastTimestamp != nil {
		diff := timestamp - *wc.lastTimestamp

		// use delta only if it is positive
		if diff >= 0 {
			timestampDelta = &diff
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
				err := wc.writeChunk(&chunk.Chunk0{
					ChunkStreamID:   msg.ChunkStreamID,
					Timestamp:       uint32(timestamp),
					Type:            msg.Type,
					MessageStreamID: msg.MessageStreamID,
					BodyLen:         (bodyLen),
					Body:            msg.Body[pos : pos+chunkBodyLen],
				})
				if err != nil {
					return err
				}

			case *wc.lastType != msg.Type || *wc.lastBodyLen != bodyLen:
				err := wc.writeChunk(&chunk.Chunk1{
					ChunkStreamID:  msg.ChunkStreamID,
					TimestampDelta: uint32(*timestampDelta),
					Type:           msg.Type,
					BodyLen:        (bodyLen),
					Body:           msg.Body[pos : pos+chunkBodyLen],
				})
				if err != nil {
					return err
				}

			case wc.lastTimestampDelta == nil || *wc.lastTimestampDelta != *timestampDelta:
				err := wc.writeChunk(&chunk.Chunk2{
					ChunkStreamID:  msg.ChunkStreamID,
					TimestampDelta: uint32(*timestampDelta),
					Body:           msg.Body[pos : pos+chunkBodyLen],
				})
				if err != nil {
					return err
				}

			default:
				err := wc.writeChunk(&chunk.Chunk3{
					ChunkStreamID: msg.ChunkStreamID,
					Body:          msg.Body[pos : pos+chunkBodyLen],
				})
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
			v4 := timestamp
			wc.lastTimestamp = &v4

			if timestampDelta != nil {
				v5 := *timestampDelta
				wc.lastTimestampDelta = &v5
			}
		} else {
			err := wc.writeChunk(&chunk.Chunk3{
				ChunkStreamID: msg.ChunkStreamID,
				Body:          msg.Body[pos : pos+chunkBodyLen],
			})
			if err != nil {
				return err
			}
		}

		pos += chunkBodyLen

		if (bodyLen - pos) == 0 {
			return wc.mw.bw.Flush()
		}
	}
}

// Writer is a raw message writer.
type Writer struct {
	bcw              *bytecounter.Writer
	bw               *bufio.Writer
	checkAcknowledge bool
	chunkSize        uint32
	ackWindowSize    uint32
	ackValue         uint32
	chunkStreams     map[byte]*writerChunkStream
}

// NewWriter allocates a Writer.
func NewWriter(
	w io.Writer,
	bcw *bytecounter.Writer,
	checkAcknowledge bool,
) *Writer {
	return &Writer{
		bcw:              bcw,
		bw:               bufio.NewWriter(w),
		checkAcknowledge: checkAcknowledge,
		chunkSize:        128,
		chunkStreams:     make(map[byte]*writerChunkStream),
	}
}

// SetChunkSize sets the maximum chunk size.
func (w *Writer) SetChunkSize(v uint32) {
	w.chunkSize = v
}

// SetWindowAckSize sets the window acknowledgement size.
func (w *Writer) SetWindowAckSize(v uint32) {
	w.ackWindowSize = v
}

// SetAcknowledgeValue sets the acknowledge sequence number.
func (w *Writer) SetAcknowledgeValue(v uint32) {
	w.ackValue = v
}

// Write writes a Message.
func (w *Writer) Write(msg *Message) error {
	wc, ok := w.chunkStreams[msg.ChunkStreamID]
	if !ok {
		wc = &writerChunkStream{mw: w}
		w.chunkStreams[msg.ChunkStreamID] = wc
	}

	return wc.writeMessage(msg)
}
