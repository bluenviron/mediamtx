// Package amf0 contains an AMF0 marshaler and unmarshaler.
package amf0

import (
	"fmt"
	"math"
)

// Marshal encodes AMF0 data.
func Marshal(data []interface{}) ([]byte, error) {
	n, err := marshalSize(data)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, n)
	n = 0

	for _, item := range data {
		n += marshalItem(item, buf[n:])
	}

	return buf, nil
}

func marshalSize(data []interface{}) (int, error) {
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
