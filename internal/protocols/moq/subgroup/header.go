// Package subgroup implements the subgroup stream.
package subgroup

import (
	"io"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

// Header is the SUBGROUP_HEADER structure.
// spec: draft-18, section 11.4.2
type Header struct {
	Properties  bool
	FirstObject bool
	TrackAlias  uint64
	GroupID     uint64
}

func (h *Header) read(r io.Reader) error {
	var b [1]byte
	_, err := r.Read(b[:])
	if err != nil {
		return err
	}
	h.Properties = (b[0] & 0x01) != 0
	h.FirstObject = (b[0] & 0x40) != 0

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

	return nil
}

func (h Header) marshalSize() int {
	st := varint.Varint(0x30)
	if h.Properties {
		st |= 0x01
	}
	if h.FirstObject {
		st |= 0x40
	}
	return st.MarshalSize() +
		varint.Varint(h.TrackAlias).MarshalSize() +
		varint.Varint(h.GroupID).MarshalSize()
}

func (h Header) marshalTo(buf []byte) int {
	st := varint.Varint(0x30)
	if h.Properties {
		st |= 0x01
	}
	if h.FirstObject {
		st |= 0x40
	}
	pos := st.MarshalTo(buf)
	pos += varint.Varint(h.TrackAlias).MarshalTo(buf[pos:])
	pos += varint.Varint(h.GroupID).MarshalTo(buf[pos:])
	return pos
}
