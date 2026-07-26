package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeRequestError varint.Varint = 0x05

// RequestErrorCode is a code of REQUEST_ERROR.
type RequestErrorCode uint64

// spec: draft-18, section 15.10.2
const (
	RequestErrorCodeUnauthorized RequestErrorCode = 0x01
	RequestErrorCodeNotSupported RequestErrorCode = 0x03
	RequestErrorCodeDoesNotExist RequestErrorCode = 0x10
	RequestErrorCodeUninterested RequestErrorCode = 0x20
)

// RequestError is the REQUEST_ERROR control message.
// spec: draft-18, section 10.6.2
type RequestError struct {
	Code   RequestErrorCode
	Reason string
}

func (*RequestError) isMessage() {}

func (m *RequestError) unmarshal(buf []byte) error {
	var code varint.Varint
	n, err := code.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	m.Code = RequestErrorCode(code)

	var retry varint.Varint
	n, err = retry.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	var l varint.Varint
	n, err = l.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	if uint64(len(buf)) < uint64(l) {
		return fmt.Errorf("not enough bytes")
	}

	m.Reason = string(buf[:l])

	return nil
}

func (m RequestError) marshalSize() int {
	payloadSize := varint.Varint(m.Code).MarshalSize() + 1 +
		varint.Varint(len(m.Reason)).MarshalSize() + len(m.Reason)
	return typeRequestError.MarshalSize() + 2 + payloadSize
}

func (m RequestError) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(m.Code).MarshalSize() + 1 +
		varint.Varint(len(m.Reason)).MarshalSize() + len(m.Reason)
	n := typeRequestError.MarshalTo(buf)
	buf[n] = byte(payloadSize >> 8)
	buf[n+1] = byte(payloadSize)
	n += 2
	n += varint.Varint(m.Code).MarshalTo(buf[n:])
	buf[n] = 0x00
	n++
	n += varint.Varint(len(m.Reason)).MarshalTo(buf[n:])
	n += copy(buf[n:], m.Reason)
	return n
}

// Marshal implements Message.
func (m RequestError) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
