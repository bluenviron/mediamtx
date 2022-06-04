package message

import (
	"fmt"
	"io"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

func messageFromType(typ chunk.MessageType) (Message, error) {
	switch typ {
	case chunk.MessageTypeSetChunkSize:
		return &MsgSetChunkSize{}, nil

	case chunk.MessageTypeSetWindowAckSize:
		return &MsgSetWindowAckSize{}, nil

	case chunk.MessageTypeSetPeerBandwidth:
		return &MsgSetPeerBandwidth{}, nil

	case chunk.MessageTypeUserControl:
		return &MsgUserControl{}, nil

	case chunk.MessageTypeCommandAMF0:
		return &MsgCommandAMF0{}, nil

	case chunk.MessageTypeDataAMF0:
		return &MsgDataAMF0{}, nil

	case chunk.MessageTypeAudio:
		return &MsgAudio{}, nil

	case chunk.MessageTypeVideo:
		return &MsgVideo{}, nil

	default:
		return nil, fmt.Errorf("unhandled message")
	}
}

// Reader is a message reader.
type Reader struct {
	r *rawmessage.Reader
}

// NewReader allocates a Reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: rawmessage.NewReader(r),
	}
}

// SetChunkSize sets the maximum chunk size.
func (r *Reader) SetChunkSize(v int) {
	r.r.SetChunkSize(v)
}

// Read reads a essage.
func (r *Reader) Read() (Message, error) {
	raw, err := r.r.Read()
	if err != nil {
		return nil, err
	}

	msg, err := messageFromType(raw.Type)
	if err != nil {
		return nil, err
	}

	err = msg.Unmarshal(raw)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
