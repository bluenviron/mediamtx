package controlmessage //nolint:dupl

import (
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typePublishOk varint.Varint = 0x1E

// PublishOk is the PUBLISH_OK control message.
// spec: draft-17, section 9.12
// spec: draft-18/19, section 10.5 (alias of REQUEST_OK)
type PublishOk struct {
	Parameters      parameter.Parameters
	TrackProperties property.Properties
}

func (*PublishOk) isMessage() {}

func (m *PublishOk) unmarshal(buf []byte) error {
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

func (m PublishOk) marshalSize() int {
	payloadSize := varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()
	return typePublishOk.MarshalSize() + 2 + payloadSize
}

func (m PublishOk) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()
	n := typePublishOk.MarshalTo(buf)
	buf[n] = byte(payloadSize >> 8)
	buf[n+1] = byte(payloadSize)
	n += 2
	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:])
	n += m.TrackProperties.MarshalTo(buf[n:])
	return n
}

// Marshal implements Message.
func (m PublishOk) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
