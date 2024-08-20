//go:build (linux && arm) || (linux && arm64)
// +build linux,arm linux,arm64

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

	for i := 0; i < nf; i++ {
		entry := rt.Field(i).Name + ":"
		f := rv.Field(i)

		switch f.Kind() {
		case reflect.Uint:
			entry += strconv.FormatUint(f.Uint(), 10)

		case reflect.Float64:
			entry += strconv.FormatFloat(f.Float(), 'f', -1, 64)

		case reflect.String:
			entry += base64.StdEncoding.EncodeToString([]byte(f.String()))

		case reflect.Bool:
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
