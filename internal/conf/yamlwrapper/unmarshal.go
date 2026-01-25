// Package yamlwrapper contains a YAML unmarshaler.
package yamlwrapper

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"gopkg.in/yaml.v2"
)

// differences with respect to the standard package:
// - unknown fields cause an error
// - integer map keys cause an error

func convertKeys(i any) (any, error) {
	switch x := i.(type) {
	case map[any]any:
		m2 := map[string]any{}
		for k, v := range x {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("integer keys are not supported (%v)", k)
			}

			var err error
			m2[ks], err = convertKeys(v)
			if err != nil {
				return nil, err
			}
		}
		return m2, nil

	case []any:
		a2 := make([]any, len(x))
		for i, v := range x {
			var err error
			a2[i], err = convertKeys(v)
			if err != nil {
				return nil, err
			}
		}
		return a2, nil
	}

	return i, nil
}

// Unmarshal loads the configuration from YAML.
func Unmarshal(buf []byte, dest any) error {
	// load YAML into a generic map.
	// "UnmarshalStrict is like Unmarshal except that any fields that are found in the data
	// that do not have corresponding struct members, or mapping keys that are duplicates, will result in an error."
	var temp any
	err := yaml.UnmarshalStrict(buf, &temp)
	if err != nil {
		return err
	}

	// convert interface{} keys into string keys to avoid JSON errors
	temp, err = convertKeys(temp)
	if err != nil {
		return err
	}

	// convert the generic map into JSON
	buf, err = json.Marshal(temp)
	if err != nil {
		return err
	}

	// load JSON into destination
	return jsonwrapper.Unmarshal(buf, dest)
}
