// Package namespace contains the MoQ namespace.
package namespace

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const (
	// spec: draft-17/18/19, section 2.4.1
	maxFieldCount = 32
)

// Namespace is a MOQ track namespace.
// spec: draft-17/18/19, section 2.4.1
type Namespace []string

// Unmarshal deserializes a namespace from a buffer.
// It returns the number of consumed bytes.
func (ns *Namespace) Unmarshal(buf []byte) (int, error) {
	origLen := len(buf)

	var nsCount varint.Varint
	n, err := nsCount.Unmarshal(buf)
	if err != nil {
		return 0, err
	}
	buf = buf[n:]

	if nsCount > maxFieldCount {
		return 0, fmt.Errorf("too many namespace fields: %d", nsCount)
	}

	v := make(Namespace, nsCount)
	for i := range v {
		var l varint.Varint
		n, err = l.Unmarshal(buf)
		if err != nil {
			return 0, err
		}
		buf = buf[n:]

		if uint64(len(buf)) < uint64(l) {
			return 0, fmt.Errorf("not enough bytes for namespace part")
		}

		v[i] = string(buf[:l])
		buf = buf[int(l):]
	}

	*ns = v
	return origLen - len(buf), nil
}

// MarshalSize returns the size of a serialized namespace.
func (ns Namespace) MarshalSize() int {
	n := varint.Varint(len(ns)).MarshalSize()
	for _, part := range ns {
		n += varint.Varint(len(part)).MarshalSize() + len(part)
	}
	return n
}

// MarshalTo serializes a namespace into a destination buffer.
func (ns Namespace) MarshalTo(buf []byte) int {
	n := varint.Varint(len(ns)).MarshalTo(buf)
	for _, part := range ns {
		n += varint.Varint(len(part)).MarshalTo(buf[n:])
		n += copy(buf[n:], part)
	}
	return n
}
