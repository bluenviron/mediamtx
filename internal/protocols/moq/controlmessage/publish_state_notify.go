package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/parameter"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typePublishStateNotify varint.Varint = 0x22

// PublishStateNotify is a unilateral publisher notification in drafts 20 and later.
type PublishStateNotify struct {
	Parameters parameter.Parameters
}

func (*PublishStateNotify) isMessage() {}

func (PublishStateNotify) String() string {
	return "PUBLISH_STATE_NOTIFY"
}

func (m *PublishStateNotify) unmarshal(buf []byte, version version.Version) error {
	var count varint.Varint
	n, err := count.Unmarshal(buf)
	if err != nil {
		return err
	}
	consumed, err := m.Parameters.Unmarshal(int(count), buf[n:], version)
	if err != nil {
		return err
	}
	if n+consumed != len(buf) {
		return fmt.Errorf("unexpected bytes after PUBLISH_STATE_NOTIFY parameters")
	}
	return nil
}

func (m PublishStateNotify) marshalSize() int {
	payloadSize := varint.Varint(len(m.Parameters)).MarshalSize() + m.Parameters.MarshalSize()

	return marshalTypeAndPayloadSizeSize(typePublishStateNotify) + payloadSize
}

func (m PublishStateNotify) marshalTo(buf []byte, versio version.Version) int {
	payloadSize := varint.Varint(len(m.Parameters)).MarshalSize() + m.Parameters.MarshalSize()

	n := marshalTypeAndPayloadSize(typePublishStateNotify, payloadSize, buf)

	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	n += m.Parameters.MarshalTo(buf[n:], versio)
	return n
}

// Marshal implements Message.
func (m PublishStateNotify) Marshal(versio version.Version) []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf, versio)
	return buf
}
