package controlmessage

import (
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeSubscribeOk varint.Varint = 0x04

// SubscribeOk is the SUBSCRIBE_OK control message.
// spec: draft-18, section 10.8
type SubscribeOk struct {
	TrackAlias      uint64
	Parameters      parameter.Parameters
	TrackProperties property.Properties
}

func (*SubscribeOk) isMessage() {}

func (m *SubscribeOk) unmarshal(buf []byte) error {
	var v varint.Varint
	n, err := v.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	m.TrackAlias = uint64(v)

	var numParams varint.Varint
	n, err = numParams.Unmarshal(buf)
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

func (m SubscribeOk) marshalSize() int {
	payloadSize := varint.Varint(m.TrackAlias).MarshalSize() +
		varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()
	return typeSubscribeOk.MarshalSize() + 2 + payloadSize
}

func (m SubscribeOk) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(m.TrackAlias).MarshalSize() +
		varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()
	n := typeSubscribeOk.MarshalTo(buf)
	buf[n] = byte(payloadSize >> 8)
	buf[n+1] = byte(payloadSize)
	n += 2
	n += varint.Varint(m.TrackAlias).MarshalTo(buf[n:])
	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:])
	n += m.TrackProperties.MarshalTo(buf[n:])
	return n
}

// Marshal implements Message.
func (m SubscribeOk) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
