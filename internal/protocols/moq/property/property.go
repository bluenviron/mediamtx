// Package property contains Media over QUIC object properties.
package property

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

// Property is an object property.
// spec: draft-18, section 11.2.1.2
type Property interface {
	isProperty()
	propType() varint.Varint
	unmarshal(buf []byte) (int, error)
	marshalSize() int
	marshalTo(buf []byte) int
}

// Properties are object properties.
// spec: draft-18, section 11.2.1.2
type Properties []Property

// Unmarshal decodes properties.
func (p *Properties) Unmarshal(buf []byte) error {
	var currentType uint64

	for len(buf) != 0 {
		var delta varint.Varint
		n, err := delta.Unmarshal(buf)
		if err != nil {
			return err
		}

		buf = buf[n:]
		currentType += uint64(delta)

		switch currentType {
		case timestampPropertyType:
			var ts Timestamp
			var n2 int
			n2, err = ts.unmarshal(buf)
			if err != nil {
				return err
			}
			*p = append(*p, &ts)
			buf = buf[n2:]

		default:
			if currentType%2 == 1 {
				var length varint.Varint
				var n2 int
				n2, err = length.Unmarshal(buf)
				if err != nil {
					return err
				}
				if len(buf)-n2 < int(length) {
					return fmt.Errorf("not enough bytes for unknown property")
				}
				buf = buf[n2+int(length):]
			} else {
				var skip varint.Varint
				var n2 int
				n2, err = skip.Unmarshal(buf)
				if err != nil {
					return err
				}
				buf = buf[n2:]
			}
		}
	}

	return nil
}

// MarshalSize returns the size of marshaled properties.
func (p Properties) MarshalSize() int {
	n := 0
	var prevType varint.Varint

	for _, prop := range p {
		t := prop.propType()
		delta := t - prevType
		n += delta.MarshalSize() + prop.marshalSize()
		prevType = t
	}

	return n
}

// MarshalTo encodes properties.
func (p Properties) MarshalTo(buf []byte) int {
	n := 0
	var prevType varint.Varint

	for _, prop := range p {
		t := prop.propType()
		delta := t - prevType
		n += delta.MarshalTo(buf[n:])
		n += prop.marshalTo(buf[n:])
		prevType = t
	}

	return n
}
