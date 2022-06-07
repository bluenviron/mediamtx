package rawmessage

import (
	"bufio"
	"errors"
	"fmt"

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
}

func (rc *readerChunkStream) read(typ byte) (*Message, error) {
	switch typ {
	case 0:
		if rc.curBody != nil {
			return nil, fmt.Errorf("received type 0 chunk but expected type 3 chunk")
		}

		var c0 chunk.Chunk0
		err := c0.Read(rc.mr.r, rc.mr.chunkSize)
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
		err := c1.Read(rc.mr.r, rc.mr.chunkSize)
		if err != nil {
			return nil, err
		}

		v2 := c1.Type
		rc.curType = &v2
		v3 := *rc.curTimestamp + c1.TimestampDelta
		rc.curTimestamp = &v3
		v4 := c1.BodyLen
		rc.curBodyLen = &v4

		if c1.BodyLen != uint32(len(c1.Body)) {
			rc.curBody = &c1.Body
			return nil, errMoreChunksNeeded
		}

		return &Message{
			Timestamp:       *rc.curTimestamp + c1.TimestampDelta,
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

		chunkBodyLen := int(*rc.curBodyLen)
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		var c2 chunk.Chunk2
		err := c2.Read(rc.mr.r, chunkBodyLen)
		if err != nil {
			return nil, err
		}

		v3 := *rc.curTimestamp + c2.TimestampDelta
		rc.curTimestamp = &v3

		if chunkBodyLen != len(c2.Body) {
			rc.curBody = &c2.Body
			return nil, errMoreChunksNeeded
		}

		return &Message{
			Timestamp:       *rc.curTimestamp + c2.TimestampDelta,
			Type:            *rc.curType,
			MessageStreamID: *rc.curMessageStreamID,
			Body:            c2.Body,
		}, nil

	default: // 3
		if rc.curTimestamp == nil {
			return nil, fmt.Errorf("received type 3 chunk without previous chunk")
		}

		if rc.curBody == nil {
			return nil, fmt.Errorf("unsupported")
		}

		chunkBodyLen := int(*rc.curBodyLen)
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		var c3 chunk.Chunk3
		err := c3.Read(rc.mr.r, chunkBodyLen)
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
}

// Reader is a raw message reader.
type Reader struct {
	r            *bufio.Reader
	chunkSize    int
	chunkStreams map[byte]*readerChunkStream
}

// NewReader allocates a Reader.
func NewReader(r *bufio.Reader) *Reader {
	return &Reader{
		r:            r,
		chunkSize:    128,
		chunkStreams: make(map[byte]*readerChunkStream),
	}
}

// SetChunkSize sets the maximum chunk size.
func (r *Reader) SetChunkSize(v int) {
	r.chunkSize = v
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

		msg, err := rc.read(typ)
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
