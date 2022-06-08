package message

import (
	"encoding/binary"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/rtmp/bytecounter"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/chunk"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/rawmessage"
)

func allocateMessage(raw *rawmessage.Message) (Message, error) {
	switch raw.Type {
	case chunk.MessageTypeSetChunkSize:
		return &MsgSetChunkSize{}, nil

	case chunk.MessageTypeAcknowledge:
		return &MsgAcknowledge{}, nil

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
func NewReader(r *bytecounter.Reader, onAckNeeded func(uint32) error) *Reader {
	return &Reader{
		r: rawmessage.NewReader(r, onAckNeeded),
	}
}

// Read reads a Message.
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

	switch tmsg := msg.(type) {
	case *MsgSetChunkSize:
		r.r.SetChunkSize(tmsg.Value)

	case *MsgSetWindowAckSize:
		r.r.SetWindowAckSize(tmsg.Value)
	}

	return msg, nil
}
