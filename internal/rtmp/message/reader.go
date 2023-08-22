package message

import (
	"fmt"
	"io"

	"github.com/bluenviron/mediamtx/internal/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

func allocateMessage(raw *rawmessage.Message) (Message, error) {
	switch Type(raw.Type) {
	case TypeSetChunkSize:
		return &SetChunkSize{}, nil

	case TypeAcknowledge:
		return &Acknowledge{}, nil

	case TypeSetWindowAckSize:
		return &SetWindowAckSize{}, nil

	case TypeSetPeerBandwidth:
		return &SetPeerBandwidth{}, nil

	case TypeUserControl:
		if len(raw.Body) < 2 {
			return nil, fmt.Errorf("not enough bytes")
		}

		userControlType := UserControlType(uint16(raw.Body[0])<<8 | uint16(raw.Body[1]))

		switch userControlType {
		case UserControlTypeStreamBegin:
			return &UserControlStreamBegin{}, nil

		case UserControlTypeStreamEOF:
			return &UserControlStreamEOF{}, nil

		case UserControlTypeStreamDry:
			return &UserControlStreamDry{}, nil

		case UserControlTypeSetBufferLength:
			return &UserControlSetBufferLength{}, nil

		case UserControlTypeStreamIsRecorded:
			return &UserControlStreamIsRecorded{}, nil

		case UserControlTypePingRequest:
			return &UserControlPingRequest{}, nil

		case UserControlTypePingResponse:
			return &UserControlPingResponse{}, nil

		default:
			return nil, fmt.Errorf("invalid user control type: %v", userControlType)
		}

	case TypeCommandAMF0:
		return &CommandAMF0{}, nil

	case TypeDataAMF0:
		return &DataAMF0{}, nil

	case TypeAudio:
		return &Audio{}, nil

	case TypeVideo:
		if len(raw.Body) < 5 {
			return nil, fmt.Errorf("not enough bytes")
		}

		if (raw.Body[0] & 0b10000000) != 0 {
			fourCC := FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])

			switch fourCC {
			case FourCCAV1, FourCCVP9, FourCCHEVC:
			default:
				return nil, fmt.Errorf("invalid fourCC: %v", fourCC)
			}

			extendedType := ExtendedType(raw.Body[0] & 0x0F)

			switch extendedType {
			case ExtendedTypeSequenceStart:
				return &ExtendedSequenceStart{}, nil

			case ExtendedTypeCodedFrames:
				return &ExtendedCodedFrames{}, nil

			case ExtendedTypeSequenceEnd:
				return &ExtendedSequenceEnd{}, nil

			case ExtendedTypeFramesX:
				return &ExtendedFramesX{}, nil

			case ExtendedTypeMetadata:
				return &ExtendedMetadata{}, nil

			case ExtendedTypeMPEG2TSSequenceStart:
				return &ExtendedMPEG2TSSequenceStart{}, nil

			default:
				return nil, fmt.Errorf("invalid extended type: %v", extendedType)
			}
		}
		return &Video{}, nil

	default:
		return nil, fmt.Errorf("invalid message type: %v", raw.Type)
	}
}

// Reader is a message reader.
type Reader struct {
	r *rawmessage.Reader
}

// NewReader allocates a Reader.
func NewReader(
	r io.Reader,
	bcr *bytecounter.Reader,
	onAckNeeded func(uint32) error,
) *Reader {
	return &Reader{
		r: rawmessage.NewReader(r, bcr, onAckNeeded),
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
	case *SetChunkSize:
		r.r.SetChunkSize(tmsg.Value)

	case *SetWindowAckSize:
		r.r.SetWindowAckSize(tmsg.Value)
	}

	return msg, nil
}
