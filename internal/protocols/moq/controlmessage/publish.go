package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typePublish varint.Varint = 0x1d

// Publish is the PUBLISH control message.
// spec: draft-18, section 10.10
type Publish struct {
	RequestID       uint64
	Namespace       []string
	TrackName       string
	TrackAlias      uint64
	Parameters      parameter.Parameters
	TrackProperties property.Properties
}

func (*Publish) isMessage() {}

func (m *Publish) unmarshal(buf []byte) error {
	var requestID varint.Varint
	n, err := requestID.Unmarshal(buf)
	if err != nil {
		return err
	}
	m.RequestID = uint64(requestID)
	buf = buf[n:]

	var nsCount varint.Varint
	n, err = nsCount.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	m.Namespace = make([]string, nsCount)
	for i := range m.Namespace {
		var l varint.Varint
		n, err = l.Unmarshal(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
		if len(buf) < int(l) {
			return fmt.Errorf("not enough bytes for namespace part")
		}
		m.Namespace[i] = string(buf[:l])
		buf = buf[int(l):]
	}

	var tnLen varint.Varint
	n, err = tnLen.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]
	if len(buf) < int(tnLen) {
		return fmt.Errorf("not enough bytes for track name")
	}
	m.TrackName = string(buf[:tnLen])
	buf = buf[int(tnLen):]

	var trackAlias varint.Varint
	n, err = trackAlias.Unmarshal(buf)
	if err != nil {
		return err
	}
	m.TrackAlias = uint64(trackAlias)
	buf = buf[n:]

	var paramCount varint.Varint
	n, err = paramCount.Unmarshal(buf)
	if err != nil {
		return err
	}
	buf = buf[n:]

	consumed, err := m.Parameters.Unmarshal(int(paramCount), buf)
	if err != nil {
		return err
	}
	buf = buf[consumed:]

	return m.TrackProperties.Unmarshal(buf)
}

func (m Publish) marshalSize() int {
	n := varint.Varint(m.RequestID).MarshalSize() +
		varint.Varint(len(m.Namespace)).MarshalSize()
	for _, part := range m.Namespace {
		n += varint.Varint(len(part)).MarshalSize() + len(part)
	}
	n += varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName)
	n += varint.Varint(m.TrackAlias).MarshalSize()
	n += varint.Varint(len(m.Parameters)).MarshalSize()
	n += m.Parameters.MarshalSize()
	n += m.TrackProperties.MarshalSize()

	return typePublish.MarshalSize() + 2 + n
}

func (m Publish) marshalTo(buf []byte) int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		varint.Varint(len(m.Namespace)).MarshalSize()
	for _, part := range m.Namespace {
		payloadSize += varint.Varint(len(part)).MarshalSize() + len(part)
	}
	payloadSize += varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName)
	payloadSize += varint.Varint(m.TrackAlias).MarshalSize()
	payloadSize += varint.Varint(len(m.Parameters)).MarshalSize()
	payloadSize += m.Parameters.MarshalSize()
	payloadSize += m.TrackProperties.MarshalSize()

	n := typePublish.MarshalTo(buf)
	buf[n] = byte(payloadSize >> 8)
	buf[n+1] = byte(payloadSize)
	n += 2
	n += varint.Varint(m.RequestID).MarshalTo(buf[n:])
	n += varint.Varint(len(m.Namespace)).MarshalTo(buf[n:])
	for _, part := range m.Namespace {
		n += varint.Varint(len(part)).MarshalTo(buf[n:])
		n += copy(buf[n:], part)
	}
	n += varint.Varint(len(m.TrackName)).MarshalTo(buf[n:])
	n += copy(buf[n:], m.TrackName)
	n += varint.Varint(m.TrackAlias).MarshalTo(buf[n:])
	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:])
	n += m.TrackProperties.MarshalTo(buf[n:])

	return n
}

// Marshal implements Message.
func (m Publish) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
