package property

import "github.com/bluenviron/mediamtx/internal/protocols/moq/varint"

const timestampPropertyType = 0x06

// Timestamp is the timestamp property.
// spec: draft-ietf-moq-loc, section 2.3.1.1
type Timestamp int64

func (t *Timestamp) isProperty() {}

func (Timestamp) propType() varint.Varint {
	return timestampPropertyType
}

func (t *Timestamp) unmarshal(buf []byte) (int, error) {
	var val varint.Varint
	n, err := val.Unmarshal(buf)
	if err != nil {
		return 0, err
	}
	*t = Timestamp(val)
	return n, nil
}

func (t Timestamp) marshalSize() int {
	return varint.Varint(t).MarshalSize()
}

func (t Timestamp) marshalTo(buf []byte) int {
	return varint.Varint(t).MarshalTo(buf)
}
