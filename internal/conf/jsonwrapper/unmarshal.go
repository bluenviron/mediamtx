// Package jsonwrapper contains a JSON unmarshaler.
package jsonwrapper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// differences with respect to the standard package:
// - JSON cannot contain unknown fields
// - existing elements of slices are never used, fixing https://github.com/golang/go/issues/21092
// - slices cannot be set to nil

func jsonFieldKey(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" || tag == "-" {
		return ""
	}
	return strings.Split(tag, ",")[0]
}

func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

func needsCustomDecode(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct || t.Kind() == reflect.Slice
}

func checkForUnknownFields(rawMap map[string]json.RawMessage, known map[string]int, path string) error {
	for k := range rawMap {
		if _, ok := known[k]; !ok {
			if path != "" {
				return fmt.Errorf("json: unknown field %q", path+"."+k)
			}
			return fmt.Errorf("json: unknown field %q", k)
		}
	}
	return nil
}

func decode(v reflect.Value, raw json.RawMessage, path string) error {
	for v.Kind() == reflect.Ptr {
		if isJSONNull(raw) {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}

		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}

		v = v.Elem()
	}

	if unm, ok := v.Addr().Interface().(json.Unmarshaler); ok {
		return unm.UnmarshalJSON(raw)
	}

	switch v.Kind() {
	case reflect.Struct:
		var rawMap map[string]json.RawMessage
		err := json.Unmarshal(raw, &rawMap)
		if err != nil {
			return err
		}

		vType := v.Type()
		known := make(map[string]int, v.NumField())

		for i := 0; i < v.NumField(); i++ {
			if key := jsonFieldKey(vType.Field(i)); key != "" {
				known[key] = i
			}
		}

		err = checkForUnknownFields(rawMap, known, path)
		if err != nil {
			return err
		}

		for key, fieldIdx := range known {
			rawVal, ok := rawMap[key]
			if !ok {
				continue
			}

			fieldPath := key
			if path != "" {
				fieldPath = path + "." + key
			}

			err = decode(v.Field(fieldIdx), rawVal, fieldPath)
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Slice:
		if isJSONNull(raw) {
			if path != "" {
				return fmt.Errorf("cannot set slice %q to nil", path)
			}
			return fmt.Errorf("cannot set slice to nil")
		}

		if !v.IsNil() {
			v.Set(reflect.Zero(v.Type()))
		}

		elemType := v.Type().Elem()

		if needsCustomDecode(elemType) {
			var rawElems []json.RawMessage
			if err := json.Unmarshal(raw, &rawElems); err != nil {
				return err
			}

			slice := reflect.MakeSlice(v.Type(), len(rawElems), len(rawElems))
			for i, re := range rawElems {
				err := decode(slice.Index(i), re, fmt.Sprintf("%s[%d]", path, i))
				if err != nil {
					return err
				}
			}
			v.Set(slice)

			return nil
		}

		return json.Unmarshal(raw, v.Addr().Interface())

	default:
		return json.Unmarshal(raw, v.Addr().Interface())
	}
}

// Unmarshal decodes JSON.
func Unmarshal(buf []byte, dest any) error {
	return Decode(bytes.NewReader(buf), dest)
}

// Decode decodes JSON.
func Decode(r io.Reader, dest any) error {
	buf, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return decode(reflect.ValueOf(dest).Elem(), buf, "")
}
