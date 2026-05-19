// Package parameter contains MoQ parameters.
package parameter

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

// Parameter is a parameter of a control message.
// spec: draft-18, section 10.2
type Parameter interface {
	isParameter()
	paramType() uint64
	unmarshal(data []byte) (int, error)
	marshalSize() int
	marshalTo(buf []byte) int
}

// Parameters is a list of parameters.
type Parameters []Parameter

// Unmarshal decodes parameters. Returns the number of bytes consumed.
func (p *Parameters) Unmarshal(count int, buf []byte) (int, error) {
	var currentType uint64
	total := 0

	for range uint64(count) {
		var typeDelta varint.Varint
		n, err := typeDelta.Unmarshal(buf)
		if err != nil {
			return 0, err
		}

		buf = buf[n:]
		total += n
		currentType += uint64(typeDelta)

		var param Parameter

		switch currentType {
		case typeAuthorizationToken:
			param = &AuthorizationToken{}

		default:
			return 0, fmt.Errorf("unsupported parameter type: %d", currentType)
		}

		n, err = param.unmarshal(buf)
		if err != nil {
			return 0, fmt.Errorf("failed to unmarshal authorization token: %w", err)
		}

		*p = append(*p, param)
		buf = buf[n:]
		total += n
	}

	return total, nil
}

// MarshalSize returns the size of marshaled parameters.
func (p Parameters) MarshalSize() int {
	n := 0
	var prevType uint64

	for _, param := range p {
		t := param.paramType()
		delta := t - prevType
		n += varint.Varint(delta).MarshalSize() + param.marshalSize()
		prevType = t
	}

	return n
}

// MarshalTo encodes parameters.
func (p Parameters) MarshalTo(buf []byte) int {
	n := 0
	var prevType uint64

	for _, param := range p {
		t := param.paramType()
		delta := t - prevType
		n += varint.Varint(delta).MarshalTo(buf[n:])
		n += param.marshalTo(buf[n:])
		prevType = t
	}

	return n
}
