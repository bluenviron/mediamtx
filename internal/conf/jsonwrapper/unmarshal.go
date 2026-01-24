// Package jsonwrapper contains a JSON unmarshaler.
package jsonwrapper

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
)

// differences with respect to the standard package:
// - unknown fields cause an error
// - using existing elements of slices is prevented, fixing https://github.com/golang/go/issues/21092

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

	nilExistingSlices(reflect.ValueOf(dest), raw)

	d := json.NewDecoder(bytes.NewReader(buf))
	d.DisallowUnknownFields()
	return d.Decode(dest)
}

// nilExistingSlices recursively nils slices that are present in the JSON data.
func nilExistingSlices(v reflect.Value, jsonData any) {
	if !v.IsValid() || jsonData == nil {
		return
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Slice:
		if _, ok := jsonData.([]any); ok {
			if !v.IsNil() {
				v.Set(reflect.Zero(v.Type()))
			}
			return
		}

	case reflect.Struct:
		jsonMap, ok := jsonData.(map[string]any)
		if !ok {
			return
		}

		vType := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := vType.Field(i)

			jsonKey := fieldType.Tag.Get("json")
			if jsonKey == "" || jsonKey == "-" {
				continue
			}
			jsonKey = strings.Split(jsonKey, ",")[0]

			if jsonValue, exists := jsonMap[jsonKey]; exists {
				if field.Kind() == reflect.Slice && !field.IsNil() {
					field.Set(reflect.Zero(field.Type()))
				}

				nilExistingSlices(field, jsonValue)
			}
		}
	}
}
