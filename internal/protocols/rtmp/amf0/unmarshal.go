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

// Unmarshal decodes AMF0 data.
func Unmarshal(buf []byte) ([]interface{}, error) {
	var out []interface{}

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
