// Package message contains a RTMP message reader/writer.
package message

import (
	"github.com/bluenviron/mediamtx/internal/rtmp/rawmessage"
)

const (
	// ControlChunkStreamID is the stream ID used for control messages.
	ControlChunkStreamID = 2
)

// Type is a message type.
type Type byte

// message types.
const (
	TypeSetChunkSize     Type = 1
	TypeAbortMessage     Type = 2
	TypeAcknowledge      Type = 3
	TypeSetWindowAckSize Type = 5
	TypeSetPeerBandwidth Type = 6

	TypeUserControl Type = 4

	TypeCommandAMF3 Type = 17
	TypeCommandAMF0 Type = 20

	TypeDataAMF3 Type = 15
	TypeDataAMF0 Type = 18

	TypeAudio Type = 8
	TypeVideo Type = 9
)

// UserControlType is a user control type.
type UserControlType uint16

// user control types.
const (
	UserControlTypeStreamBegin      UserControlType = 0
	UserControlTypeStreamEOF        UserControlType = 1
	UserControlTypeStreamDry        UserControlType = 2
	UserControlTypeSetBufferLength  UserControlType = 3
	UserControlTypeStreamIsRecorded UserControlType = 4
	UserControlTypePingRequest      UserControlType = 6
	UserControlTypePingResponse     UserControlType = 7
)

// ExtendedType is a message extended type.
type ExtendedType uint8

// message extended types.
const (
	ExtendedTypeSequenceStart        ExtendedType = 0
	ExtendedTypeCodedFrames          ExtendedType = 1
	ExtendedTypeSequenceEnd          ExtendedType = 2
	ExtendedTypeFramesX              ExtendedType = 3
	ExtendedTypeMetadata             ExtendedType = 4
	ExtendedTypeMPEG2TSSequenceStart ExtendedType = 5
)

// FourCC is an identifier of a video codec.
type FourCC uint32

// video codec identifiers.
var (
	FourCCAV1  FourCC = 'a'<<24 | 'v'<<16 | '0'<<8 | '1'
	FourCCVP9  FourCC = 'v'<<24 | 'p'<<16 | '0'<<8 | '9'
	FourCCHEVC FourCC = 'h'<<24 | 'v'<<16 | 'c'<<8 | '1'
)

// Message is a message.
type Message interface {
	Unmarshal(*rawmessage.Message) error
	Marshal() (*rawmessage.Message, error)
}
