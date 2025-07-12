package rawmessage

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/chunk"
)

var errMoreChunksNeeded = errors.New("more chunks are needed")

const (
	maxBodySize = 10 * 1024 * 1024
)

func joinFragments(fragments [][]byte, size uint32) []byte {
	ret := make([]byte, size)
	n := 0
	for _, p := range fragments {
		n += copy(ret[n:], p)
	}
	return ret
}

type readerChunkStream struct {
	mr                    *Reader
	curTimestamp          uint32
	curTimestampAvailable bool
	curType               uint8
	curMessageStreamID    uint32
	curBodyLen            uint32
	curBodyFragments      [][]byte
	curBodyRecv           uint32
	curTimestampDelta     uint32
	hasExtendedTimestamp  bool
}

func (rc *readerChunkStream) readChunk(c chunk.Chunk, bodySize uint32, hasExtendedTimestamp bool) error {
	err := c.Read(rc.mr.br, bodySize, hasExtendedTimestamp)
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
		if rc.curBodyRecv != 0 {
			return nil, fmt.Errorf("received type 0 chunk but expected type 3 chunk")
		}

		err := rc.readChunk(&rc.mr.c0, rc.mr.chunkSize, false)
		if err != nil {
			return nil, err
		}

		rc.curMessageStreamID = rc.mr.c0.MessageStreamID
		rc.curType = rc.mr.c0.Type
		rc.curTimestamp = rc.mr.c0.Timestamp
		rc.curTimestampAvailable = true
		rc.curTimestampDelta = 0
		rc.curBodyLen = rc.mr.c0.BodyLen
		rc.hasExtendedTimestamp = rc.mr.c0.Timestamp >= 0xFFFFFF

		if rc.curBodyLen > maxBodySize {
			return nil, fmt.Errorf("body size (%d) exceeds maximum (%d)", rc.curBodyLen, maxBodySize)
		}

		le := uint32(len(rc.mr.c0.Body))

		if rc.mr.c0.BodyLen != le {
			rc.curBodyFragments = append(rc.curBodyFragments, rc.mr.c0.Body)
			rc.curBodyRecv = le
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(rc.mr.c0.Timestamp) * time.Millisecond
		rc.mr.msg.Type = rc.mr.c0.Type
		rc.mr.msg.MessageStreamID = rc.mr.c0.MessageStreamID
		rc.mr.msg.Body = rc.mr.c0.Body
		return rc.mr.msg.clone(), nil

	case 1:
		if !rc.curTimestampAvailable {
			return nil, fmt.Errorf("received type 1 chunk without previous chunk")
		}

		if rc.curBodyRecv != 0 {
			return nil, fmt.Errorf("received type 1 chunk but expected type 3 chunk")
		}

		err := rc.readChunk(&rc.mr.c1, rc.mr.chunkSize, false)
		if err != nil {
			return nil, err
		}

		rc.curType = rc.mr.c1.Type
		rc.curTimestamp += rc.mr.c1.TimestampDelta
		rc.curTimestampDelta = rc.mr.c1.TimestampDelta
		rc.curBodyLen = rc.mr.c1.BodyLen
		rc.hasExtendedTimestamp = rc.mr.c1.TimestampDelta >= 0xFFFFFF

		if rc.curBodyLen > maxBodySize {
			return nil, fmt.Errorf("body size (%d) exceeds maximum (%d)", rc.curBodyLen, maxBodySize)
		}

		le := uint32(len(rc.mr.c1.Body))

		if rc.mr.c1.BodyLen != le {
			rc.curBodyFragments = append(rc.curBodyFragments, rc.mr.c1.Body)
			rc.curBodyRecv = le
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(rc.curTimestamp) * time.Millisecond
		rc.mr.msg.Type = rc.mr.c1.Type
		rc.mr.msg.MessageStreamID = rc.curMessageStreamID
		rc.mr.msg.Body = rc.mr.c1.Body
		return rc.mr.msg.clone(), nil

	case 2:
		if !rc.curTimestampAvailable {
			return nil, fmt.Errorf("received type 2 chunk without previous chunk")
		}

		if rc.curBodyRecv != 0 {
			return nil, fmt.Errorf("received type 2 chunk but expected type 3 chunk")
		}

		chunkBodyLen := rc.curBodyLen
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		err := rc.readChunk(&rc.mr.c2, chunkBodyLen, false)
		if err != nil {
			return nil, err
		}

		rc.curTimestamp += rc.mr.c2.TimestampDelta
		rc.curTimestampDelta = rc.mr.c2.TimestampDelta
		rc.hasExtendedTimestamp = rc.mr.c2.TimestampDelta >= 0xFFFFFF

		le := uint32(len(rc.mr.c2.Body))

		if rc.curBodyLen != le {
			rc.curBodyFragments = append(rc.curBodyFragments, rc.mr.c2.Body)
			rc.curBodyRecv = le
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(rc.curTimestamp) * time.Millisecond
		rc.mr.msg.Type = rc.curType
		rc.mr.msg.MessageStreamID = rc.curMessageStreamID
		rc.mr.msg.Body = rc.mr.c2.Body
		return rc.mr.msg.clone(), nil

	default: // 3
		if rc.curBodyRecv != 0 {
			chunkBodyLen := rc.curBodyLen - rc.curBodyRecv
			if chunkBodyLen > rc.mr.chunkSize {
				chunkBodyLen = rc.mr.chunkSize
			}

			err := rc.readChunk(&rc.mr.c3, chunkBodyLen, rc.hasExtendedTimestamp)
			if err != nil {
				return nil, err
			}

			rc.curBodyFragments = append(rc.curBodyFragments, rc.mr.c3.Body)
			rc.curBodyRecv += uint32(len(rc.mr.c3.Body))

			if rc.curBodyLen != rc.curBodyRecv {
				return nil, errMoreChunksNeeded
			}

			rc.mr.msg.Timestamp = time.Duration(rc.curTimestamp) * time.Millisecond
			rc.mr.msg.Type = rc.curType
			rc.mr.msg.MessageStreamID = rc.curMessageStreamID
			rc.mr.msg.Body = joinFragments(rc.curBodyFragments, rc.curBodyRecv)
			rc.curBodyFragments = rc.curBodyFragments[:0]
			rc.curBodyRecv = 0
			return rc.mr.msg.clone(), nil
		}

		if !rc.curTimestampAvailable {
			return nil, fmt.Errorf("received type 3 chunk without previous chunk")
		}

		chunkBodyLen := rc.curBodyLen
		if chunkBodyLen > rc.mr.chunkSize {
			chunkBodyLen = rc.mr.chunkSize
		}

		err := rc.readChunk(&rc.mr.c3, chunkBodyLen, rc.hasExtendedTimestamp)
		if err != nil {
			return nil, err
		}

		rc.curTimestamp += rc.curTimestampDelta

		le := uint32(len(rc.mr.c3.Body))

		if rc.curBodyLen != le {
			rc.curBodyFragments = append(rc.curBodyFragments, rc.mr.c3.Body)
			rc.curBodyRecv = le
			return nil, errMoreChunksNeeded
		}

		rc.mr.msg.Timestamp = time.Duration(rc.curTimestamp) * time.Millisecond
		rc.mr.msg.Type = rc.curType
		rc.mr.msg.MessageStreamID = rc.curMessageStreamID
		rc.mr.msg.Body = rc.mr.c3.Body
		return rc.mr.msg.clone(), nil
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
func (r *Reader) SetChunkSize(v uint32) error {
	if v > maxBodySize {
		return fmt.Errorf("chunk size (%d) exceeds maximum (%d)", v, maxBodySize)
	}

	r.chunkSize = v
	return nil
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

		if chunkStreamID < 2 {
			return nil, fmt.Errorf("extended chunk stream IDs are not supported (yet)")
		}

		rc, ok := r.chunkStreams[chunkStreamID]
		if !ok {
			rc = &readerChunkStream{mr: r}
			r.chunkStreams[chunkStreamID] = rc
		}

		r.br.UnreadByte() //nolint:errcheck

		msg, err := rc.readMessage(typ)
		if err != nil {
			if errors.Is(err, errMoreChunksNeeded) {
				continue
			}
			return nil, err
		}

		msg.ChunkStreamID = chunkStreamID

		return msg, err
	}
}
