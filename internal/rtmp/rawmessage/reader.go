package rawmessage

import (
	"errors"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
)

var errMoreChunksNeeded = errors.New("more chunks are needed")

type readerChunkStream struct {
	mr                 *Reader
	curTimestamp       *uint32
	curType            *chunk.MessageType
	curMessageStreamID *uint32
	curBodyLen         *uint32
	curBody            *[]byte
	curTimestampDelta  *uint32
}

func (rc *readerChunkStream) readChunk(c chunk.Chunk, chunkBodySize uint32) error {
	err := c.Read(rc.mr.r, chunkBodySize)
	if err != nil {
		return err
	}

	// check if an ack is needed
	if rc.mr.ackWindowSize != 0 {
		count := rc.mr.r.Count()
		diff := count - rc.mr.lastAckCount
		// TODO: handle overflow

		if diff > (rc.mr.ackWindowSize) {
			err := rc.mr.onAckNeeded(count)
			if err != nil {
				return err
			}

			rc.mr.lastAckCount += (rc.mr.ackWindowSize)
		}
	}

	return nil
}

func (rc *readerChunkStream) readMessage(typ byte) (*Message, error) {
	switch typ {
	case 0:
		if rc.curBody != nil {
			return nil, fmt.Errorf("received type 0 chunk but expected type 3 chunk")
		}

		var c0 chunk.Chunk0
		err := rc.readChunk(&c0, rc.mr.chunkSize)
		if err != nil {
			return nil, err
		}

		v1 := c0.MessageStreamID
		rc.curMessageStreamID = &v1
		v2 := c0.Type
		rc.curType = &v2
		v3 := c0.Timestamp
		rc.curTimestamp = &v3
		v4 := c0.BodyLen
		rc.curBodyLen = &v4
		rc.curTimestampDelta = nil

		if c0.BodyLen != uint32(len(c0.Body)) {
			rc.curBody = &c0.Body
			return nil, errMoreChunksNeeded
		}

		return &Message{
			Timestamp:       c0.Timestamp,
			Type:            c0.Type,
			MessageStreamID: c0.MessageStreamID,
			Body:            c0.Body,
		}, nil

	case 1:
		if rc.curTimestamp == nil {
			return nil, fmt.Errorf("received type 1 chunk without previous chunk")
		}

		if rc.curBody != nil {
			return nil, fmt.Errorf("received type 1 chunk but expected type 3 chunk")
		}

		var c1 chunk.Chunk1
		err := rc.readChunk(&c1, rc.mr.chunkSize)
		if err != nil {
			return nil, err
		}

		v2 := c1.Type
		rc.curType = &v2
		v3 := *rc.curTimestamp + c1.TimestampDelta
		rc.curTimestamp = &v3
		v4 := c1.BodyLen
		rc.curBodyLen = &v4
		v5 := c1.TimestampDelta
		rc.curTimestampDelta = &v5

		if c1.BodyLen != uint32(len(c1.Body)) {
			rc.curBody = &c1.Body
			return nil, errMoreChunksNeeded
		}

		return &Message{
			Timestamp:       *rc.curTimestamp,
			Type:            c1.Type,
			MessageStreamID: *rc.curMessageStreamID,
			Body:            c1.Body,
		}, nil

	case 2:
		if rc.curTimestamp == nil {
			return nil, fmt.Errorf("received type 2 chunk without previous chunk")
		}

		if rc.curBody != nil {
			return nil, fmt.Errorf("received type 2 chunk but expected type 3 chunk")
		}

		chunkBodyLen := (*rc.curBodyLen)
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		var c2 chunk.Chunk2
		err := rc.readChunk(&c2, chunkBodyLen)
		if err != nil {
			return nil, err
		}

		v1 := *rc.curTimestamp + c2.TimestampDelta
		rc.curTimestamp = &v1
		v2 := c2.TimestampDelta
		rc.curTimestampDelta = &v2

		if chunkBodyLen != uint32(len(c2.Body)) {
			rc.curBody = &c2.Body
			return nil, errMoreChunksNeeded
		}

		return &Message{
			Timestamp:       *rc.curTimestamp,
			Type:            *rc.curType,
			MessageStreamID: *rc.curMessageStreamID,
			Body:            c2.Body,
		}, nil

	default: // 3
		if rc.curBody == nil && rc.curTimestampDelta == nil {
			return nil, fmt.Errorf("received type 3 chunk without previous chunk")
		}

		if rc.curBody != nil {
			chunkBodyLen := (*rc.curBodyLen) - uint32(len(*rc.curBody))
			if chunkBodyLen > rc.mr.chunkSize {
				chunkBodyLen = rc.mr.chunkSize
			}

			var c3 chunk.Chunk3
			err := rc.readChunk(&c3, chunkBodyLen)
			if err != nil {
				return nil, err
			}

			*rc.curBody = append(*rc.curBody, c3.Body...)

			if *rc.curBodyLen != uint32(len(*rc.curBody)) {
				return nil, errMoreChunksNeeded
			}

			body := *rc.curBody
			rc.curBody = nil

			return &Message{
				Timestamp:       *rc.curTimestamp,
				Type:            *rc.curType,
				MessageStreamID: *rc.curMessageStreamID,
				Body:            body,
			}, nil
		}

		chunkBodyLen := (*rc.curBodyLen)
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		var c3 chunk.Chunk3
		err := rc.readChunk(&c3, chunkBodyLen)
		if err != nil {
			return nil, err
		}

		v1 := *rc.curTimestamp + *rc.curTimestampDelta
		rc.curTimestamp = &v1

		return &Message{
			Timestamp:       *rc.curTimestamp,
			Type:            *rc.curType,
			MessageStreamID: *rc.curMessageStreamID,
			Body:            c3.Body,
		}, nil
	}
}

// Reader is a raw message reader.
type Reader struct {
	r           *bytecounter.Reader
	onAckNeeded func(uint32) error

	chunkSize     uint32
	ackWindowSize uint32
	lastAckCount  uint32
	chunkStreams  map[byte]*readerChunkStream
}

// NewReader allocates a Reader.
func NewReader(r *bytecounter.Reader, onAckNeeded func(uint32) error) *Reader {
	return &Reader{
		r:            r,
		onAckNeeded:  onAckNeeded,
		chunkSize:    128,
		chunkStreams: make(map[byte]*readerChunkStream),
	}
}

// SetChunkSize sets the maximum chunk size.
func (r *Reader) SetChunkSize(v uint32) {
	r.chunkSize = v
}

// SetWindowAckSize sets the window acknowledgement size.
func (r *Reader) SetWindowAckSize(v uint32) {
	r.ackWindowSize = v
}

// Read reads a Message.
func (r *Reader) Read() (*Message, error) {
	for {
		byt, err := r.r.ReadByte()
		if err != nil {
			return nil, err
		}

		typ := byt >> 6
		chunkStreamID := byt & 0x3F

		rc, ok := r.chunkStreams[chunkStreamID]
		if !ok {
			rc = &readerChunkStream{mr: r}
			r.chunkStreams[chunkStreamID] = rc
		}

		r.r.UnreadByte()

		msg, err := rc.readMessage(typ)
		if err != nil {
			if err == errMoreChunksNeeded {
				continue
			}
			return nil, err
		}

		msg.ChunkStreamID = chunkStreamID

		return msg, err
	}
}
