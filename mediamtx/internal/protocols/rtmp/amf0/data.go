// Package amf0 contains an AMF0 decoder and encoder.
package amf0

import (
	"errors"
	"fmt"
	"math"
)

const (
	markerNumber      = 0x00
	markerBoolean     = 0x01
	markerString      = 0x02
	markerObject      = 0x03
	markerMovieclip   = 0x04
	markerNull        = 0x05
	markerUndefined   = 0x06
	markerReference   = 0x07
	markerECMAArray   = 0x08
	markerObjectEnd   = 0x09
	markerStrictArray = 0x0A
	markerDate        = 0x0B
	markerLongString  = 0x0C
	markerUnsupported = 0x0D
	markerRecordset   = 0x0E
	markerXMLDocument = 0xF
	markerTypedObject = 0x10
)

var errBufferTooShort = errors.New("buffer is too short")

// StrictArray is an AMF0 Strict Array.
type StrictArray []interface{}

// Data is a list of ActionScript object graphs.
type Data []interface{}

// Unmarshal decodes AMF0 data.
func Unmarshal(buf []byte) (Data, error) {
	var out Data

	for len(buf) != 0 {
		var item interface{}
		var err error
		item, buf, err = unmarshal(buf)
		if err != nil {
			return nil, err
		}

		out = append(out, item)
	}

	return out, nil
}

func unmarshal(buf []byte) (interface{}, []byte, error) {
	if len(buf) < 1 {
		return nil, nil, errBufferTooShort
	}

	var marker byte
	marker, buf = buf[0], buf[1:]

	switch marker {
	case markerNumber:
		if len(buf) < 8 {
			return nil, nil, errBufferTooShort
		}

		return math.Float64frombits(uint64(buf[0])<<56 | uint64(buf[1])<<48 | uint64(buf[2])<<40 | uint64(buf[3])<<32 |
			uint64(buf[4])<<24 | uint64(buf[5])<<16 | uint64(buf[6])<<8 | uint64(buf[7])), buf[8:], nil

	case markerBoolean:
		if len(buf) < 1 {
			return nil, nil, errBufferTooShort
		}

		return (buf[0] != 0), buf[1:], nil

	case markerString:
		if len(buf) < 2 {
			return nil, nil, errBufferTooShort
		}

		le := uint16(buf[0])<<8 | uint16(buf[1])
		buf = buf[2:]

		if len(buf) < int(le) {
			return nil, nil, errBufferTooShort
		}

		return string(buf[:le]), buf[le:], nil

	case markerECMAArray:
		if len(buf) < 4 {
			return nil, nil, errBufferTooShort
		}

		buf = buf[4:]

		out := ECMAArray{}

		for {
			if len(buf) < 2 {
				return nil, nil, errBufferTooShort
			}

			keyLen := uint16(buf[0])<<8 | uint16(buf[1])
			buf = buf[2:]

			if keyLen == 0 {
				break
			}

			if len(buf) < int(keyLen) {
				return nil, nil, errBufferTooShort
			}

			key := string(buf[:keyLen])
			buf = buf[keyLen:]

			var value interface{}
			var err error
			value, buf, err = unmarshal(buf)
			if err != nil {
				return nil, nil, err
			}

			out = append(out, ObjectEntry{
				Key:   key,
				Value: value,
			})
		}

		if len(buf) < 1 {
			return nil, nil, errBufferTooShort
		}

		if buf[0] != markerObjectEnd {
			return nil, nil, fmt.Errorf("object end not found")
		}

		return out, buf[1:], nil

	case markerObject:
		out := Object{}

		for {
			if len(buf) < 2 {
				return nil, nil, errBufferTooShort
			}

			keyLen := uint16(buf[0])<<8 | uint16(buf[1])
			buf = buf[2:]

			if keyLen == 0 {
				break
			}

			if len(buf) < int(keyLen) {
				return nil, nil, errBufferTooShort
			}

			key := string(buf[:keyLen])
			buf = buf[keyLen:]

			var value interface{}
			var err error
			value, buf, err = unmarshal(buf)
			if err != nil {
				return nil, nil, err
			}

			out = append(out, ObjectEntry{
				Key:   key,
				Value: value,
			})
		}

		if len(buf) < 1 {
			return nil, nil, errBufferTooShort
		}

		if buf[0] != markerObjectEnd {
			return nil, nil, fmt.Errorf("object end not found")
		}

		return out, buf[1:], nil

	case markerNull:
		return nil, buf, nil

	case markerStrictArray:
		if len(buf) < 4 {
			return nil, nil, errBufferTooShort
		}

		arrayCount := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		buf = buf[4:]

		out := StrictArray{}

		for i := 0; i < int(arrayCount); i++ {
			var value interface{}
			var err error
			value, buf, err = unmarshal(buf)
			if err != nil {
				return nil, nil, err
			}

			out = append(out, value)
		}

		return out, buf, nil

	default:
		return nil, nil, fmt.Errorf("unsupported marker 0x%.2x", marker)
	}
}

// Marshal encodes AMF0 data.
func (data Data) Marshal() ([]byte, error) {
	n, err := data.MarshalSize()
	if err != nil {
		return nil, err
	}

	buf := make([]byte, n)
	_, err = data.MarshalTo(buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// MarshalTo encodes AMF0 data into an existing buffer.
func (data Data) MarshalTo(buf []byte) (int, error) {
	n := 0

	for _, item := range data {
		n += marshalItem(item, buf[n:])
	}

	return n, nil
}

// MarshalSize returns the size needed to encode data in AMF0.
func (data Data) MarshalSize() (int, error) {
	n := 0

	for _, item := range data {
		in, err := marshalSizeItem(item)
		if err != nil {
			return 0, err
		}

		n += in
	}

	return n, nil
}

func marshalSizeItem(item interface{}) (int, error) {
	switch item := item.(type) {
	case float64:
		return 9, nil

	case bool:
		return 2, nil

	case string:
		return 3 + len(item), nil

	case ECMAArray:
		n := 5

		for _, entry := range item {
			en, err := marshalSizeItem(entry.Value)
			if err != nil {
				return 0, err
			}

			n += 2 + len(entry.Key) + en
		}

		n += 3

		return n, nil

	case Object:
		n := 1

		for _, entry := range item {
			en, err := marshalSizeItem(entry.Value)
			if err != nil {
				return 0, err
			}

			n += 2 + len(entry.Key) + en
		}

		n += 3

		return n, nil

	case StrictArray:
		n := 5

		for _, entry := range item {
			en, err := marshalSizeItem(entry)
			if err != nil {
				return 0, err
			}

			n += en
		}

		return n, nil

	case nil:
		return 1, nil

	default:
		return 0, fmt.Errorf("unsupported data type: %T", item)
	}
}

func marshalItem(item interface{}, buf []byte) int {
	switch item := item.(type) {
	case float64:
		v := math.Float64bits(item)
		buf[0] = markerNumber
		buf[1] = byte(v >> 56)
		buf[2] = byte(v >> 48)
		buf[3] = byte(v >> 40)
		buf[4] = byte(v >> 32)
		buf[5] = byte(v >> 24)
		buf[6] = byte(v >> 16)
		buf[7] = byte(v >> 8)
		buf[8] = byte(v)
		return 9

	case bool:
		buf[0] = markerBoolean
		if item {
			buf[1] = 1
		}
		return 2

	case string:
		le := len(item)
		buf[0] = markerString
		buf[1] = byte(le >> 8)
		buf[2] = byte(le)
		copy(buf[3:], item)
		return 3 + le

	case ECMAArray:
		le := len(item)
		buf[0] = markerECMAArray
		buf[1] = byte(le >> 24)
		buf[2] = byte(le >> 16)
		buf[3] = byte(le >> 8)
		buf[4] = byte(le)
		n := 5

		for _, entry := range item {
			le := len(entry.Key)
			buf[n] = byte(le >> 8)
			buf[n+1] = byte(le)
			copy(buf[n+2:], entry.Key)
			n += 2 + le

			n += marshalItem(entry.Value, buf[n:])
		}

		buf[n] = 0
		buf[n+1] = 0
		buf[n+2] = markerObjectEnd

		return n + 3

	case Object:
		buf[0] = markerObject
		n := 1

		for _, entry := range item {
			le := len(entry.Key)
			buf[n] = byte(le >> 8)
			buf[n+1] = byte(le)
			copy(buf[n+2:], entry.Key)
			n += 2 + le

			n += marshalItem(entry.Value, buf[n:])
		}

		buf[n] = 0
		buf[n+1] = 0
		buf[n+2] = markerObjectEnd

		return n + 3

	case StrictArray:
		le := len(item)
		buf[0] = markerStrictArray
		buf[1] = byte(le >> 24)
		buf[2] = byte(le >> 16)
		buf[3] = byte(le >> 8)
		buf[4] = byte(le)
		n := 5

		for _, entry := range item {
			n += marshalItem(entry, buf[n:])
		}

		return n

	default:
		buf[0] = markerNull
		return 1
	}
}
