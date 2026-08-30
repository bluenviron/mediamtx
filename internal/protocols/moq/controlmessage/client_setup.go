package controlmessage

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

const typeClientSetup varint.Varint = 0x20

// ClientSetup is the CLIENT_SETUP control message.
// spec: draft-16, section 9.3
type ClientSetup Setup

func (*ClientSetup) isMessage() {}

func (m *ClientSetup) unmarshal(buf []byte) error {
	return (*Setup)(m).unmarshal(buf)
}

// Marshal implements Message.
func (m ClientSetup) Marshal() []byte {
	s := Setup(m)
	buf := make([]byte, s.marshalSize(typeClientSetup))
	s.marshalTo(buf, typeClientSetup)
	return buf
}
