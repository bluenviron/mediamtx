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

// Marshal implements Message.
func (m PublishStateNotify) Marshal(versio version.Version) []byte {
	length := varint.Varint(len(m.Parameters)).MarshalSize() + m.Parameters.MarshalSize()
	buf := make([]byte, typePublishStateNotify.MarshalSize()+2+length)
	n := typePublishStateNotify.MarshalTo(buf)
	buf[n] = byte(length >> 8)
	buf[n+1] = byte(length)
	n += 2
	n += varint.Varint(len(m.Parameters)).MarshalTo(buf[n:])
	m.Parameters.MarshalTo(buf[n:], versio)
	return buf
}
