//go:build (linux && arm) || (linux && arm64)

package rpicamera

import (
	"encoding/base64"
	"reflect"
	"strconv"
	"strings"
)

func (p params) serialize() []byte {
	rv := reflect.ValueOf(p)
	rt := rv.Type()
	nf := rv.NumField()
	ret := make([]string, nf)

	for i := range nf {
		entry := rt.Field(i).Name + ":"
		f := rv.Field(i)
		v := f.Interface()

		switch v := v.(type) {
		case uint32:
			entry += strconv.FormatUint(uint64(v), 10)

		case float32:
			entry += strconv.FormatFloat(float64(v), 'f', -1, 64)

		case string:
			entry += base64.StdEncoding.EncodeToString([]byte(v))

		case bool:
			if f.Bool() {
				entry += "1"
			} else {
				entry += "0"
			}

		default:
			panic("unhandled type")
		}

		ret[i] = entry
	}

	return []byte(strings.Join(ret, " "))
}
