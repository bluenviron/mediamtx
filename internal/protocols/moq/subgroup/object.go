package subgroup

import (
	"fmt"
	"io"

	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/varint"
)

const (
	maxPropsLen    = 128 * 1024
	maxPayloadSize = 10 * 1024 * 1024
)

// Object is an object of a subgroup stream.
// spec: draft-18, section 11.4.2
type Object struct {
	IDDelta    uint64
	Properties property.Properties
	Payload    []byte
}

func (o *Object) read(r io.Reader, header *Header) error {
	var idDelta varint.Varint
	if err := idDelta.Read(r); err != nil {
		return err
	}

	o.IDDelta = uint64(idDelta)

	if header.Properties {
		var propsLen varint.Varint
		err := propsLen.Read(r)
		if err != nil {
			return err
		}

		if propsLen > 0 {
			if propsLen > maxPropsLen {
				return fmt.Errorf("properties too large: %d", propsLen)
			}

			props := make([]byte, propsLen)
			_, err = io.ReadFull(r, props)
			if err != nil {
				return err
			}

			err = o.Properties.Unmarshal(props)
			if err != nil {
				return err
			}
		}
	}

	var payloadLen varint.Varint
	err := payloadLen.Read(r)
	if err != nil {
		return err
	}

	if payloadLen == 0 {
		var status varint.Varint
		if err = status.Read(r); err != nil {
			return err
		}

		if status != 0x03 && status != 0x04 {
			return fmt.Errorf("unexpected status: 0x%x", status)
		}

		return nil
	}

	if payloadLen > maxPayloadSize {
		return fmt.Errorf("payload too large: %d", payloadLen)
	}

	o.Payload = make([]byte, payloadLen)
	_, err = io.ReadFull(r, o.Payload)
	if err != nil {
		return err
	}

	return nil
}

func (o Object) marshalSize(header *Header) int {
	propsFieldSize := 0
	if header.Properties {
		propsLen := o.Properties.MarshalSize()
		propsFieldSize = varint.Varint(propsLen).MarshalSize() + propsLen
	}

	if len(o.Payload) == 0 {
		return varint.Varint(o.IDDelta).MarshalSize() + propsFieldSize + 2
	}

	return varint.Varint(o.IDDelta).MarshalSize() +
		propsFieldSize +
		varint.Varint(len(o.Payload)).MarshalSize() +
		len(o.Payload)
}

func (o Object) marshalTo(buf []byte, header *Header) int {
	n := varint.Varint(o.IDDelta).MarshalTo(buf)

	if header.Properties {
		propsLen := o.Properties.MarshalSize()
		n += varint.Varint(propsLen).MarshalTo(buf[n:])
		n += o.Properties.MarshalTo(buf[n:])
	}

	if len(o.Payload) == 0 {
		buf[n] = 0x00
		n++
		buf[n] = 0x03
		n++
		return n
	}

	n += varint.Varint(len(o.Payload)).MarshalTo(buf[n:])
	n += copy(buf[n:], o.Payload)

	return n
}
