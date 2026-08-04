package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const typeSetup varint.Varint = 0x2F00

const (
	setupOptionPath      varint.Varint = 0x01
	setupOptionAuthority varint.Varint = 0x05
)

// Setup is the SETUP control message.
// spec: draft-18/19, section 10.3
type Setup struct {
	Path      string
	Authority string
}

func (*Setup) isMessage() {}

func (m *Setup) unmarshal(buf []byte) error {
	var previousType uint64
	for len(buf) > 0 {
		var deltaType varint.Varint
		n, err := deltaType.Unmarshal(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]

		currentType := previousType + uint64(deltaType)
		previousType = currentType

		if currentType%2 == 0 { // even type: single varint value
			var v varint.Varint
			n, err = v.Unmarshal(buf)
			if err != nil {
				return err
			}
			buf = buf[n:]
			continue
		}

		// odd type: length-prefixed byte field
		var l varint.Varint
		n, err = l.Unmarshal(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]

		if uint64(len(buf)) < uint64(l) {
			return fmt.Errorf("not enough bytes for setup option")
		}

		value := string(buf[:int(l)])
		buf = buf[int(l):]

		switch varint.Varint(currentType) {
		case setupOptionPath:
			m.Path = value

		case setupOptionAuthority:
			m.Authority = value
		}
	}

	return nil
}

func (m Setup) marshalSize() int {
	payloadSize := 0
	var previousType varint.Varint

	if m.Path != "" {
		delta := setupOptionPath - previousType
		payloadSize += delta.MarshalSize() +
			varint.Varint(len(m.Path)).MarshalSize() +
			len(m.Path)
		previousType = setupOptionPath
	}

	if m.Authority != "" {
		delta := setupOptionAuthority - previousType
		payloadSize += delta.MarshalSize() +
			varint.Varint(len(m.Authority)).MarshalSize() +
			len(m.Authority)
	}

	return typeSetup.MarshalSize() + 2 + payloadSize
}

func (m Setup) marshalTo(buf []byte) int {
	payloadSize := m.marshalSize() - typeSetup.MarshalSize() - 2

	pos := typeSetup.MarshalTo(buf)
	buf[pos] = byte(payloadSize >> 8)
	buf[pos+1] = byte(payloadSize)
	pos += 2

	var previousType varint.Varint

	if m.Path != "" {
		delta := setupOptionPath - previousType
		pos += delta.MarshalTo(buf[pos:])
		pos += varint.Varint(len(m.Path)).MarshalTo(buf[pos:])
		pos += copy(buf[pos:], m.Path)
		previousType = setupOptionPath
	}

	if m.Authority != "" {
		delta := setupOptionAuthority - previousType
		pos += delta.MarshalTo(buf[pos:])
		pos += varint.Varint(len(m.Authority)).MarshalTo(buf[pos:])
		pos += copy(buf[pos:], m.Authority)
	}

	return pos
}

// Marshal implements Message.
func (m Setup) Marshal() []byte {
	buf := make([]byte, m.marshalSize())
	m.marshalTo(buf)
	return buf
}
