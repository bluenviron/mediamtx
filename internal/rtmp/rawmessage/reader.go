package rawmessage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/chunk"
)

var errMoreChunksNeeded = errors.New("more chunks are needed")

type readerChunkStream struct {
	mr                 *Reader
	curTimestamp       *uint32
	curType            *uint8
	curMessageStreamID *uint32
	curBodyLen         *uint32
	curBody            []byte
	curTimestampDelta  *uint32
}

func (rc *readerChunkStream) readChunk(c chunk.Chunk, chunkBodySize uint32) error {
	err := c.Read(rc.mr.br, chunkBodySize)
	if err != nil {
		return err
	}

	// check if an ack is needed
	if rc.mr.ackWindowSize != 0 {
		count := uint32(rc.mr.bcr.Count())
		diff := count - rc.mr.lastAckCount

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

		err := rc.readChunk(&rc.mr.c0, rc.mr.chunkSize)
		if err != nil {
			return nil, err
		}

		v1 := rc.mr.c0.MessageStreamID
		rc.curMessageStreamID = &v1
		v2 := rc.mr.c0.Type
		rc.curType = &v2
		v3 := rc.mr.c0.Timestamp
		rc.curTimestamp = &v3
		v4 := rc.mr.c0.BodyLen
		rc.curBodyLen = &v4
		rc.curTimestampDelta = nil

		if rc.mr.c0.BodyLen != uint32(len(rc.mr.c0.Body)) {
			rc.curBody = rc.mr.c0.Body
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(rc.mr.c0.Timestamp) * time.Millisecond
		rc.mr.msg.Type = rc.mr.c0.Type
		rc.mr.msg.MessageStreamID = rc.mr.c0.MessageStreamID
		rc.mr.msg.Body = rc.mr.c0.Body
		return &rc.mr.msg, nil

	case 1:
		if rc.curTimestamp == nil {
			return nil, fmt.Errorf("received type 1 chunk without previous chunk")
		}

		if rc.curBody != nil {
			return nil, fmt.Errorf("received type 1 chunk but expected type 3 chunk")
		}

		err := rc.readChunk(&rc.mr.c1, rc.mr.chunkSize)
		if err != nil {
			return nil, err
		}

		v2 := rc.mr.c1.Type
		rc.curType = &v2
		v3 := *rc.curTimestamp + rc.mr.c1.TimestampDelta
		rc.curTimestamp = &v3
		v4 := rc.mr.c1.BodyLen
		rc.curBodyLen = &v4
		v5 := rc.mr.c1.TimestampDelta
		rc.curTimestampDelta = &v5

		if rc.mr.c1.BodyLen != uint32(len(rc.mr.c1.Body)) {
			rc.curBody = rc.mr.c1.Body
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(*rc.curTimestamp) * time.Millisecond
		rc.mr.msg.Type = rc.mr.c1.Type
		rc.mr.msg.MessageStreamID = *rc.curMessageStreamID
		rc.mr.msg.Body = rc.mr.c1.Body
		return &rc.mr.msg, nil

	case 2:
		if rc.curTimestamp == nil {
			return nil, fmt.Errorf("received type 2 chunk without previous chunk")
		}

		if rc.curBody != nil {
			return nil, fmt.Errorf("received type 2 chunk but expected type 3 chunk")
		}

		chunkBodyLen := *rc.curBodyLen
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		err := rc.readChunk(&rc.mr.c2, chunkBodyLen)
		if err != nil {
			return nil, err
		}

		v1 := *rc.curTimestamp + rc.mr.c2.TimestampDelta
		rc.curTimestamp = &v1
		v2 := rc.mr.c2.TimestampDelta
		rc.curTimestampDelta = &v2

		if *rc.curBodyLen != uint32(len(rc.mr.c2.Body)) {
			rc.curBody = rc.mr.c2.Body
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(*rc.curTimestamp) * time.Millisecond
		rc.mr.msg.Type = *rc.curType
		rc.mr.msg.MessageStreamID = *rc.curMessageStreamID
		rc.mr.msg.Body = rc.mr.c2.Body
		return &rc.mr.msg, nil

	default: // 3
		if rc.curBody == nil && rc.curTimestampDelta == nil {
			return nil, fmt.Errorf("received type 3 chunk without previous chunk")
		}

		if rc.curBody != nil {
			chunkBodyLen := (*rc.curBodyLen) - uint32(len(rc.curBody))
			if chunkBodyLen > rc.mr.chunkSize {
				chunkBodyLen = rc.mr.chunkSize
			}

			err := rc.readChunk(&rc.mr.c3, chunkBodyLen)
			if err != nil {
				return nil, err
			}

			rc.curBody = append(rc.curBody, rc.mr.c3.Body...)

			if *rc.curBodyLen != uint32(len(rc.curBody)) {
				return nil, errMoreChunksNeeded
			}

			body := rc.curBody
			rc.curBody = nil

			rc.mr.msg.Timestamp = time.Duration(*rc.curTimestamp) * time.Millisecond
			rc.mr.msg.Type = *rc.curType
			rc.mr.msg.MessageStreamID = *rc.curMessageStreamID
			rc.mr.msg.Body = body
			return &rc.mr.msg, nil
		}

		chunkBodyLen := (*rc.curBodyLen)
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		err := rc.readChunk(&rc.mr.c3, chunkBodyLen)
		if err != nil {
			return nil, err
		}

		v1 := *rc.curTimestamp + *rc.curTimestampDelta
		rc.curTimestamp = &v1

		if *rc.curBodyLen != uint32(len(rc.mr.c3.Body)) {
			rc.curBody = rc.mr.c3.Body
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(*rc.curTimestamp) * time.Millisecond
		rc.mr.msg.Type = *rc.curType
		rc.mr.msg.MessageStreamID = *rc.curMessageStreamID
		rc.mr.msg.Body = rc.mr.c3.Body
		return &rc.mr.msg, nil
	}
}

// Reader is a raw message reader.
type Reader struct {
	bcr         *bytecounter.Reader
	onAckNeeded func(uint32) error

	br            *bufio.Reader
	chunkSize     uint32
	ackWindowSize uint32
	lastAckCount  uint32
	msg           Message
	c0            chunk.Chunk0
	c1            chunk.Chunk1
	c2            chunk.Chunk2
	c3            chunk.Chunk3
	chunkStreams  map[byte]*readerChunkStream
}

// NewReader allocates a Reader.
func NewReader(
	r io.Reader,
	bcr *bytecounter.Reader,
	onAckNeeded func(uint32) error,
) *Reader {
	return &Reader{
		bcr:          bcr,
		br:           bufio.NewReader(r),
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
		byt, err := r.br.ReadByte()
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

		r.br.UnreadByte() //nolint:errcheck

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
