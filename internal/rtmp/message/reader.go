package message

import (
	"bufio"
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

func allocateMessage(raw *rawmessage.Message) (Message, error) {
	switch raw.Type {
	case chunk.MessageTypeSetChunkSize:
		return &MsgSetChunkSize{}, nil

	case chunk.MessageTypeSetWindowAckSize:
		return &MsgSetWindowAckSize{}, nil

	case chunk.MessageTypeSetPeerBandwidth:
		return &MsgSetPeerBandwidth{}, nil

	case chunk.MessageTypeUserControl:
		if len(raw.Body) < 2 {
			return nil, fmt.Errorf("invalid body size")
		}

		subType := binary.BigEndian.Uint16(raw.Body)
		switch subType {
		case UserControlTypeStreamBegin:
			return &MsgUserControlStreamBegin{}, nil

		case UserControlTypeStreamEOF:
			return &MsgUserControlStreamEOF{}, nil

		case UserControlTypeStreamDry:
			return &MsgUserControlStreamDry{}, nil

		case UserControlTypeSetBufferLength:
			return &MsgUserControlSetBufferLength{}, nil

		case UserControlTypeStreamIsRecorded:
			return &MsgUserControlStreamIsRecorded{}, nil

		case UserControlTypePingRequest:
			return &MsgUserControlPingRequest{}, nil

		case UserControlTypePingResponse:
			return &MsgUserControlPingResponse{}, nil

		default:
			return nil, fmt.Errorf("invalid user control type")
		}

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
func NewReader(r *bufio.Reader) *Reader {
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

	msg, err := allocateMessage(raw)
	if err != nil {
		return nil, err
	}

	err = msg.Unmarshal(raw)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
