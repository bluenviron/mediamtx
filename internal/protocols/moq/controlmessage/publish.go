package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/namespace"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typePublish varint.Varint = 0x1d

// Publish is the PUBLISH control message.
// spec:
// * draft-17, section 9.11
// * draft-18/19, section 10.10
type Publish struct {
	RequestID       uint64
	Namespace       namespace.Namespace
	TrackName       string
	TrackAlias      uint64
	Parameters      parameter.Parameters
	TrackProperties property.Properties
}

func (*Publish) isMessage() {}

func (m Publish) String() string {
	return fmt.Sprintf("PUBLISH requestID=%d namespace=%s trackName=%s trackAlias=%d",
		m.RequestID, m.Namespace, m.TrackName, m.TrackAlias)
}

func (m *Publish) unmarshal(buf []byte, version version.Version) error {
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

	var trackAlias varint.Varint
	n, err = trackAlias.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	m.TrackAlias = uint64(trackAlias)

	var paramCount varint.Varint
	n, err = paramCount.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	consumed, err := m.Parameters.Unmarshal(int(paramCount), buf, version)
	if err != nil {
		return err
	}
	buf = buf[consumed:]

	return m.TrackProperties.Unmarshal(buf)
}

func (m Publish) marshalSize() int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		m.Namespace.MarshalSize() +
		varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName) +
		varint.Varint(m.TrackAlias).MarshalSize() +
		varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()

	return marshalTypeAndPayloadSizeSize(typePublish) + payloadSize
}

func (m Publish) marshalTo(buf []byte, versio version.Version) int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		m.Namespace.MarshalSize() +
		varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName) +
		varint.Varint(m.TrackAlias).MarshalSize() +
		varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize() +
		m.TrackProperties.MarshalSize()

	n := marshalTypeAndPayloadSize(typePublish, payloadSize, buf)

	n += varint.Varint(m.RequestID).MarshalTo(buf[n:])
	n += m.Namespace.MarshalTo(buf[n:])
	n += varint.Varint(len(m.TrackName)).MarshalTo(buf[n:])
	n += copy(buf[n:], m.TrackName)
	n += varint.Varint(m.TrackAlias).MarshalTo(buf[n:])
	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:], versio)
	n += m.TrackProperties.MarshalTo(buf[n:])

	return n
}

// Marshal implements Message.
func (m Publish) Marshal(versio version.Version) []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf, versio)
	return buf
}
