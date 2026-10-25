package controlmessage

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeSetup varint.Varint = 0x2F00

const (
	setupOptionPath      varint.Varint = 0x01
	setupOptionAuthority varint.Varint = 0x05
)

// Setup is the SETUP control message.
// spec:
// * draft-17, section 9.4
// * draft-18/19, section 10.3
type Setup struct {
	Path      string
	Authority string
}

func (*Setup) isMessage() {}

func (m Setup) String() string {
	return fmt.Sprintf("SETUP path=%s authority=%s", m.Path, m.Authority)
}

func (m *Setup) unmarshal(buf []byte, _ version.Version) error {
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

func (m Setup) marshalSize(t varint.Varint) int {
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

	return marshalTypeAndPayloadSizeSize(t) + payloadSize
}

func (m Setup) marshalTo(buf []byte, t varint.Varint) int {
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

	n := marshalTypeAndPayloadSize(t, payloadSize, buf)

	previousType = 0

	if m.Path != "" {
		delta := setupOptionPath - previousType
		n += delta.MarshalTo(buf[n:])
		n += varint.Varint(len(m.Path)).MarshalTo(buf[n:])
		n += copy(buf[n:], m.Path)
		previousType = setupOptionPath
	}

	if m.Authority != "" {
		delta := setupOptionAuthority - previousType
		n += delta.MarshalTo(buf[n:])
		n += varint.Varint(len(m.Authority)).MarshalTo(buf[n:])
		n += copy(buf[n:], m.Authority)
	}

	return n
}

// Marshal implements Message.
func (m Setup) Marshal(_ version.Version) []byte {
	buf := make([]byte, m.marshalSize(typeSetup))
	m.marshalTo(buf, typeSetup)
	return buf
}
