package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/namespace"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeSubscribe varint.Varint = 0x03

// Subscribe is the SUBSCRIBE control message.
// spec:
// * draft-17, section 9.8
// * draft-18/19, section 10.7
type Subscribe struct {
	RequestID  uint64
	Namespace  namespace.Namespace
	TrackName  string
	Parameters parameter.Parameters
}

func (*Subscribe) isMessage() {}

func (m Subscribe) String() string {
	return fmt.Sprintf("SUBSCRIBE requestID=%d namespace=%s trackName=%s", m.RequestID, m.Namespace, m.TrackName)
}

func (m *Subscribe) unmarshal(buf []byte, version version.Version) error {
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

	n, err = m.Parameters.Unmarshal(int(paramCount), buf, version)
	if err != nil {
		return err
	}

	if n != len(buf) {
		return fmt.Errorf("unexpected bytes after SUBSCRIBE parameters")
	}

	return nil
}

func (m Subscribe) marshalSize() int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		m.Namespace.MarshalSize() +
		varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName) +
		varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize()

	return marshalTypeAndPayloadSizeSize(typeSubscribe) + payloadSize
}

func (m Subscribe) marshalTo(buf []byte, versio version.Version) int {
	payloadSize := varint.Varint(m.RequestID).MarshalSize() +
		m.Namespace.MarshalSize() +
		varint.Varint(len(m.TrackName)).MarshalSize() + len(m.TrackName) +
		varint.Varint(len(m.Parameters)).MarshalSize() +
		m.Parameters.MarshalSize()

	n := marshalTypeAndPayloadSize(typeSubscribe, payloadSize, buf)

	n += varint.Varint(m.RequestID).MarshalTo(buf[n:])
	n += m.Namespace.MarshalTo(buf[n:])
	n += varint.Varint(len(m.TrackName)).MarshalTo(buf[n:])
	n += copy(buf[n:], m.TrackName)

	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:], versio)

	return n
}

// Marshal implements Message.
func (m Subscribe) Marshal(versio version.Version) []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf, versio)
	return buf
}
