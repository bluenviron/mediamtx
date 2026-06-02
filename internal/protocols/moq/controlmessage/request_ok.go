package controlmessage

import (
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeRequestOk varint.Varint = 0x07

// RequestOk is the REQUEST_OK control message.
// spec: draft-18, section 10.5
type RequestOk struct {
	Parameters      parameter.Parameters
	TrackProperties property.Properties
}

func (*RequestOk) isMessage() {}

func (m *RequestOk) unmarshal(buf []byte) error {
	var numParams varint.Varint
	n, err := numParams.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	consumed, err := m.Parameters.Unmarshal(int(numParams), buf)
	if err != nil {
		return err
	}
	buf = buf[consumed:]

	return m.TrackProperties.Unmarshal(buf)
}

func (m RequestOk) marshalSize() int {
	payloadSize := varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()
	return typeRequestOk.MarshalSize() + 2 + payloadSize
}

func (m RequestOk) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()
	n := typeRequestOk.MarshalTo(buf)
	buf[n] = byte(payloadSize >> 8)
	buf[n+1] = byte(payloadSize)
	n += 2
	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:])
	n += m.TrackProperties.MarshalTo(buf[n:])
	return n
}

// Marshal implements Message.
func (m RequestOk) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
