package controlmessage

import (
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeClientSetup varint.Varint = 0x20

// ClientSetup is the CLIENT_SETUP control message.
// spec: draft-16, section 9.3
type ClientSetup Setup

func (*ClientSetup) isMessage() {}

func (m ClientSetup) String() string {
	return "CLIENT_SETUP path=" + m.Path + " authority=" + m.Authority
}

func (m *ClientSetup) unmarshal(buf []byte, version version.Version) error {
	return (*Setup)(m).unmarshal(buf, version)
}

// Marshal implements Message.
func (m ClientSetup) Marshal(_ version.Version) []byte {
	s := Setup(m)
	buf := make([]byte, s.marshalSize(typeClientSetup))
	s.marshalTo(buf, typeClientSetup)
	return buf
}
