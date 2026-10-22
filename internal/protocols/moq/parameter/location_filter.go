package parameter

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/version"
)

const typeLocationFilter = 0x21

// LocationFilter selects objects by location in drafts 20 and later.
type LocationFilter struct {
	Type   uint64
	Fields []uint64
}

func (*LocationFilter) isParameter() {}

func (*LocationFilter) paramType() uint64 { return typeLocationFilter }

func (p *LocationFilter) unmarshal(buf []byte, versio version.Version) (int, error) {
	var first varint.Varint
	n, err := first.Unmarshal(buf)
	if err != nil {
		return 0, err
	}

	if versio >= version.Draft22 {
		counts := []int{0, 1, 2, 3, 4, 0}
		if first >= varint.Varint(len(counts)) {
			return 0, fmt.Errorf("invalid location filter type: %d", first)
		}
		p.Type = uint64(first)
		for range counts[first] {
			var field varint.Varint
			used, readErr := field.Unmarshal(buf[n:])
			if readErr != nil {
				return 0, readErr
			}
			p.Fields = append(p.Fields, uint64(field))
			n += used
		}
		return n, nil
	}

	if uint64(len(buf[n:])) < uint64(first) {
		return 0, fmt.Errorf("invalid location filter length: %d", first)
	}
	end := n + int(first)
	for n < end {
		if len(p.Fields) == 4 {
			return 0, fmt.Errorf("too many location filter fields")
		}
		var field varint.Varint
		used, readErr := field.Unmarshal(buf[n:end])
		if readErr != nil {
			return 0, readErr
		}
		p.Fields = append(p.Fields, uint64(field))
		n += used
	}
	return n, nil
}

func (p LocationFilter) marshalSize(versio version.Version) int {
	n := 0
	for _, field := range p.Fields {
		n += varint.Varint(field).MarshalSize()
	}
	if versio >= version.Draft22 {
		return varint.Varint(p.Type).MarshalSize() + n
	}
	return varint.Varint(n).MarshalSize() + n
}

func (p LocationFilter) marshalTo(buf []byte, versio version.Version) int {
	var n int
	if versio >= version.Draft22 {
		n = varint.Varint(p.Type).MarshalTo(buf)
	} else {
		length := 0
		for _, field := range p.Fields {
			length += varint.Varint(field).MarshalSize()
		}
		n = varint.Varint(length).MarshalTo(buf)
	}

	for _, field := range p.Fields {
		n += varint.Varint(field).MarshalTo(buf[n:])
	}

	return n
}
