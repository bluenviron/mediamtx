package controlmessage

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

const typeServerSetup varint.Varint = 0x21

// ServerSetup is the SERVER_SETUP control message.
// spec: draft-16, section 9.3
type ServerSetup Setup

func (*ServerSetup) isMessage() {}

func (m *ServerSetup) unmarshal(buf []byte) error {
	return (*Setup)(m).unmarshal(buf)
}

// Marshal implements Message.
func (m ServerSetup) Marshal() []byte {
	s := Setup(m)
	buf := make([]byte, s.marshalSize(typeServerSetup))
	s.marshalTo(buf, typeServerSetup)
	return buf
}
