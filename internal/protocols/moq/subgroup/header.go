// Package subgroup implements the subgroup stream.
package subgroup

import (
	"fmt"
	"io"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

// Header is the SUBGROUP_HEADER structure.
// spec:
// * draft-17, section 10.4.2
// * draft-18/19, section 11.4.2
type Header struct {
	HasProperties bool
	IsFirstObject bool
	TrackAlias    uint64
	GroupID       uint64
}

func (h *Header) read(r io.Reader) error {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	if err != nil {
		return err
	}

	if b[0]&0x90 != 0x10 || b[0]&0x06 == 0x06 {
		return fmt.Errorf("invalid subgroup type: 0x%x", b[0])
	}

	h.HasProperties = (b[0] & 0x01) != 0
	h.IsFirstObject = (b[0] & 0x40) != 0
	hasSubgroupID := b[0]&0x06 == 0x04
	hasPriority := b[0]&0x20 == 0

	var trackAlias varint.Varint
	err = trackAlias.Read(r)
	if err != nil {
		return err
	}
	h.TrackAlias = uint64(trackAlias)

	var groupID varint.Varint
	err = groupID.Read(r)
	if err != nil {
		return err
	}
	h.GroupID = uint64(groupID)

	if hasSubgroupID {
		var subgroupID varint.Varint
		err = subgroupID.Read(r)
		if err != nil {
			return err
		}
	}

	if hasPriority {
		var priority [1]byte
		_, err = io.ReadFull(r, priority[:])
		if err != nil {
			return err
		}
	}

	return nil
}

func (h Header) marshalSize() int {
	st := varint.Varint(0x30)
	if h.HasProperties {
		st |= 0x01
	}
	if h.IsFirstObject {
		st |= 0x40
	}
	return st.MarshalSize() +
		varint.Varint(h.TrackAlias).MarshalSize() +
		varint.Varint(h.GroupID).MarshalSize()
}

func (h Header) marshalTo(buf []byte) int {
	st := varint.Varint(0x30)
	if h.HasProperties {
		st |= 0x01
	}
	if h.IsFirstObject {
		st |= 0x40
	}
	pos := st.MarshalTo(buf)
	pos += varint.Varint(h.TrackAlias).MarshalTo(buf[pos:])
	pos += varint.Varint(h.GroupID).MarshalTo(buf[pos:])
	return pos
}
