package parameter

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeFillParameters = 0x23

// FillParameters requests a fill fetch stream, which the live server does not support.
type FillParameters struct {
	Value []byte
}

func (*FillParameters) isParameter() {}

func (*FillParameters) paramType() uint64 { return typeFillParameters }

func (p *FillParameters) unmarshal(buf []byte, _ version.Version) (int, error) {
	var length varint.Varint
	n, err := length.Unmarshal(buf)
	if err != nil {
		return 0, err
	}
	if uint64(len(buf[n:])) < uint64(length) {
		return 0, fmt.Errorf("invalid fill parameters length: %d", length)
	}
	p.Value = append(p.Value[:0], buf[n:n+int(length)]...)
	return n + int(length), nil
}

func (p FillParameters) marshalSize(_ version.Version) int {
	return varint.Varint(len(p.Value)).MarshalSize() + len(p.Value)
}

func (p FillParameters) marshalTo(buf []byte, _ version.Version) int {
	n := varint.Varint(len(p.Value)).MarshalTo(buf)
	return n + copy(buf[n:], p.Value)
}
