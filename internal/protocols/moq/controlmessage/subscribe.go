package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/namespace"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeSubscribe varint.Varint = 0x03

// Subscribe is the SUBSCRIBE control message.
// spec: draft-18/19, section 10.7
type Subscribe struct {
	RequestID  uint64
	Namespace  namespace.Namespace
	TrackName  string
	Parameters parameter.Parameters
}

func (*Subscribe) isMessage() {}

func (m *Subscribe) unmarshal(buf []byte) error {
	var requestID varint.Varint
	n, err := requestID.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	m.RequestID = uint64(requestID)

	n, err = m.Namespace.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	var tnLen varint.Varint
	n, err = tnLen.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	if uint64(len(buf)) < uint64(tnLen) {
		return fmt.Errorf("invalid track name length: %d", tnLen)
	}

	m.TrackName = string(buf[:tnLen])
	buf = buf[int(tnLen):]

	var paramCount varint.Varint
	n, err = paramCount.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	_, err = m.Parameters.Unmarshal(int(paramCount), buf)
	if err != nil {
		return err
	}

	return nil
}

func (m Subscribe) marshalSize() int {
	n := varint.Varint(m.RequestID).MarshalSize() +
		m.Namespace.MarshalSize()
	n += varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName)
	n += varint.Varint(len(m.Parameters)).MarshalSize()
	n += m.Parameters.MarshalSize()

	return typeSubscribe.MarshalSize() + 2 + n
}

func (m Subscribe) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		m.Namespace.MarshalSize()
	payloadSize += varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName)
	payloadSize += varint.Varint(len(m.Parameters)).MarshalSize()
	payloadSize += m.Parameters.MarshalSize()

	n := typeSubscribe.MarshalTo(buf)
	buf[n] = byte(payloadSize >> 8)
	buf[n+1] = byte(payloadSize)
	n += 2
	n += varint.Varint(m.RequestID).MarshalTo(buf[n:])
	n += m.Namespace.MarshalTo(buf[n:])
	n += varint.Varint(len(m.TrackName)).MarshalTo(buf[n:])
	n += copy(buf[n:], m.TrackName)

	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:])

	return n
}

// Marshal implements Message.
func (m Subscribe) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
