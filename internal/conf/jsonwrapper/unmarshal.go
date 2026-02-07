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
// - prevents setting unknown fields
// - prevents using existing elements of slices, fixing https://github.com/golang/go/issues/21092
// - prevents setting slices to nil

func process(v reflect.Value, raw any, path string) error {
	switch v.Kind() {
	case reflect.Slice:
		if raw == nil {
			if path != "" {
				return fmt.Errorf("cannot set slice '%s' to nil", path)
			}
			return fmt.Errorf("cannot set slice to nil")
		}

		// nil existing slice to prevent reuse of elements
		if !v.IsNil() {
			v.Set(reflect.Zero(v.Type()))
		}

	case reflect.Struct:
		if rawMap, ok := raw.(map[string]any); ok {
			vType := v.Type()
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				fieldType := vType.Field(i)

				jsonKey := fieldType.Tag.Get("json")
				if jsonKey == "" || jsonKey == "-" {
					continue
				}
				jsonKey = strings.Split(jsonKey, ",")[0]

				if rawVal, ok2 := rawMap[jsonKey]; ok2 {
					fieldPath := jsonKey
					if path != "" {
						fieldPath = path + "." + jsonKey
					}
					err := process(field, rawVal, fieldPath)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
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

	var raw any
	err = json.Unmarshal(buf, &raw)
	if err != nil {
		return err
	}

	err = process(reflect.ValueOf(dest).Elem(), raw, "")
	if err != nil {
		return err
	}

	d := json.NewDecoder(bytes.NewReader(buf))
	d.DisallowUnknownFields()
	return d.Decode(dest)
}
