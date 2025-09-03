// Package message contains a RTMP message reader/writer.
package message

import (
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
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
	TypeUserControl      Type = 4
	TypeSetWindowAckSize Type = 5
	TypeSetPeerBandwidth Type = 6
	TypeAudio            Type = 8
	TypeVideo            Type = 9
	TypeDataAMF3         Type = 15
	TypeDataAMF0         Type = 18
	TypeCommandAMF3      Type = 17
	TypeCommandAMF0      Type = 20
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

// AudioExType is an audio message extended type.
type AudioExType uint8

// audio message extended types.
const (
	AudioExTypeSequenceStart      AudioExType = 0
	AudioExTypeCodedFrames        AudioExType = 1
	AudioExTypeSequenceEnd        AudioExType = 2
	AudioExTypeMultichannelConfig AudioExType = 4
	AudioExTypeMultitrack         AudioExType = 5
)

// VideoExType is a video message extended type.
type VideoExType uint8

// video message extended types.
const (
	VideoExTypeSequenceStart        VideoExType = 0
	VideoExTypeCodedFrames          VideoExType = 1
	VideoExTypeSequenceEnd          VideoExType = 2
	VideoExTypeFramesX              VideoExType = 3
	VideoExTypeMetadata             VideoExType = 4
	VideoExTypeMPEG2TSSequenceStart VideoExType = 5
	VideoExTypeMultitrack           VideoExType = 6
)

// FourCC is an identifier of a Extended-RTMP codec.
type FourCC uint32

// codec identifiers.
var (
	// video
	FourCCAV1  FourCC = 'a'<<24 | 'v'<<16 | '0'<<8 | '1'
	FourCCVP9  FourCC = 'v'<<24 | 'p'<<16 | '0'<<8 | '9'
	FourCCHEVC FourCC = 'h'<<24 | 'v'<<16 | 'c'<<8 | '1'
	FourCCAVC  FourCC = 'a'<<24 | 'v'<<16 | 'c'<<8 | '1'

	// audio
	FourCCOpus FourCC = 'O'<<24 | 'p'<<16 | 'u'<<8 | 's'
	FourCCAC3  FourCC = 'a'<<24 | 'c'<<16 | '-'<<8 | '3'
	FourCCMP4A FourCC = 'm'<<24 | 'p'<<16 | '4'<<8 | 'a'
	FourCCMP3  FourCC = '.'<<24 | 'm'<<16 | 'p'<<8 | '3'
)

// Message is a message.
type Message interface {
	unmarshal(*rawmessage.Message) error
	marshal() (*rawmessage.Message, error)
}
