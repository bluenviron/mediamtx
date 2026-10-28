package controlmessage //nolint:dupl

import (
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeRequestOk varint.Varint = 0x07

// RequestOk is the REQUEST_OK control message.
// spec:
// * draft-17, section 9.6
// * draft-18/19, section 10.5
type RequestOk struct {
	Parameters      parameter.Parameters
	TrackProperties property.Properties
}

func (*RequestOk) isMessage() {}

func (RequestOk) String() string {
	return "REQUEST_OK"
}

func (m *RequestOk) unmarshal(buf []byte, version version.Version) error {
	var numParams varint.Varint
	n, err := numParams.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	consumed, err := m.Parameters.Unmarshal(int(numParams), buf, version)
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

	return marshalTypeAndPayloadSizeSize(typeRequestOk) + payloadSize
}

func (m RequestOk) marshalTo(buf []byte, versio version.Version) int {
	payloadSize := varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()

	n := marshalTypeAndPayloadSize(typeRequestOk, payloadSize, buf)

	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:], versio)
	n += m.TrackProperties.MarshalTo(buf[n:])
	return n
}

// Marshal implements Message.
func (m RequestOk) Marshal(versio version.Version) []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf, versio)
	return buf
}
