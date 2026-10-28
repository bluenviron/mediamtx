package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeSubscribeOk varint.Varint = 0x04

// SubscribeOk is the SUBSCRIBE_OK control message.
// spec:
// * draft-17, section 9.9
// * draft-18/19, section 10.8
type SubscribeOk struct {
	TrackAlias      uint64
	Parameters      parameter.Parameters
	TrackProperties property.Properties
}

func (*SubscribeOk) isMessage() {}

func (m SubscribeOk) String() string {
	return fmt.Sprintf("SUBSCRIBE_OK trackAlias=%d", m.TrackAlias)
}

func (m *SubscribeOk) unmarshal(buf []byte, version version.Version) error {
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

	consumed, err := m.Parameters.Unmarshal(int(numParams), buf, version)
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

	return marshalTypeAndPayloadSizeSize(typeSubscribeOk) + payloadSize
}

func (m SubscribeOk) marshalTo(buf []byte, versio version.Version) int {
	payloadSize := varint.Varint(m.TrackAlias).MarshalSize() +
		varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()

	n := marshalTypeAndPayloadSize(typeSubscribeOk, payloadSize, buf)

	n += varint.Varint(m.TrackAlias).MarshalTo(buf[n:])
	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:], versio)
	n += m.TrackProperties.MarshalTo(buf[n:])
	return n
}

// Marshal implements Message.
func (m SubscribeOk) Marshal(versio version.Version) []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf, versio)
	return buf
}
