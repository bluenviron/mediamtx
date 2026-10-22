// Package controlmessage contains MoQ control messages.
package controlmessage

import (
	"fmt"
	"io"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

// Message is a control message.
type Message interface {
	// Marshal encodes the message to bytes.
	Marshal(versio version.Version) []byte
	String() string

	isMessage()
	unmarshal(buf []byte, version version.Version) error
}

// Read reads a Message from a Reader.
func Read(r io.Reader, version version.Version) (Message, error) {
	var t varint.Varint
	if err := t.Read(r); err != nil {
		return nil, err
	}

	var lenBuf [2]byte
	_, err := io.ReadFull(r, lenBuf[:])
	if err != nil {
		return nil, err
	}
	length := uint16(lenBuf[0])<<8 | uint16(lenBuf[1])

	payload := make([]byte, length)
	_, err = io.ReadFull(r, payload)
	if err != nil {
		return nil, err
	}

	var m Message

	switch t {
	case typeSetup:
		m = &Setup{}
	case typeClientSetup:
		m = &ClientSetup{}
	case typeServerSetup:
		m = &ServerSetup{}
	case typeSubscribe:
		m = &Subscribe{}
	case typeSubscribeOk:
		m = &SubscribeOk{}
	case typeRequestError:
		m = &RequestError{}
	case typePublish:
		m = &Publish{}
	case typePublishOk:
		m = &PublishOk{}
	case typeRequestOk:
		m = &RequestOk{}
	case typePublishStateNotify:
		m = &PublishStateNotify{}
	default:
		return nil, fmt.Errorf("unknown message type: 0x%x", t)
	}

	err = m.unmarshal(payload, version)
	if err != nil {
		return nil, err
	}

	return m, nil
}
