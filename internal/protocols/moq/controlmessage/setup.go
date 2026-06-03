package controlmessage

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

const typeSetup varint.Varint = 0x2F00

// Setup is the SETUP control message.
// spec: draft-18, section 10.3
type Setup struct{}

func (*Setup) isMessage() {}

func (*Setup) unmarshal(_ []byte) error { return nil }

func (Setup) marshalSize() int {
	return typeSetup.MarshalSize() + 2
}

func (Setup) marshalTo(buf []byte) int {
	pos := typeSetup.MarshalTo(buf)
	buf[pos] = 0x00
	buf[pos+1] = 0x00
	return pos + 2
}

// Marshal implements Message.
func (Setup) Marshal() []byte {
	buf := make([]byte, Setup{}.marshalSize())
	Setup{}.marshalTo(buf)
	return buf
}
