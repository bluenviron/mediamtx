// Package env contains a function to load configuration from environment.
package env

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type envUnmarshaler interface {
	UnmarshalEnv(string) error
}

func envHasAtLeastAKeyWithPrefix(env map[string]string, prefix string) bool {
	for key := range env {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func loadEnvInternal(env map[string]string, prefix string, rv reflect.Value) error {
	rt := rv.Type()

	if i, ok := rv.Addr().Interface().(envUnmarshaler); ok {
		if ev, ok := env[prefix]; ok {
			err := i.UnmarshalEnv(ev)
			if err != nil {
				return fmt.Errorf("%s: %s", prefix, err)
			}
		}
		return nil
	}

	switch rt {
	case reflect.TypeOf(""):
		if ev, ok := env[prefix]; ok {
			rv.SetString(ev)
		}
		return nil

	case reflect.TypeOf(int(0)):
		if ev, ok := env[prefix]; ok {
			iv, err := strconv.ParseInt(ev, 10, 32)
			if err != nil {
				return fmt.Errorf("%s: %s", prefix, err)
			}
			rv.SetInt(iv)
		}
		return nil

	case reflect.TypeOf(uint64(0)):
		if ev, ok := env[prefix]; ok {
			iv, err := strconv.ParseUint(ev, 10, 32)
			if err != nil {
				return fmt.Errorf("%s: %s", prefix, err)
			}
			rv.SetUint(iv)
		}
		return nil

	case reflect.TypeOf(float64(0)):
		if ev, ok := env[prefix]; ok {
			iv, err := strconv.ParseFloat(ev, 64)
			if err != nil {
				return fmt.Errorf("%s: %s", prefix, err)
			}
			rv.SetFloat(iv)
		}
		return nil

	case reflect.TypeOf(bool(false)):
		if ev, ok := env[prefix]; ok {
			switch strings.ToLower(ev) {
			case "yes", "true":
				rv.SetBool(true)

			case "no", "false":
				rv.SetBool(false)

			default:
				return fmt.Errorf("%s: invalid value '%s'", prefix, ev)
			}
		}
		return nil
	}

	switch rt.Kind() {
	case reflect.Map:
		for k := range env {
			if !strings.HasPrefix(k, prefix+"_") {
				continue
			}

			mapKey := strings.Split(k[len(prefix+"_"):], "_")[0]
			if len(mapKey) == 0 {
				continue
			}

			// allow only keys in uppercase
			if mapKey != strings.ToUpper(mapKey) {
				continue
			}

			// initialize only if there's at least one key
			if rv.IsNil() {
				rv.Set(reflect.MakeMap(rt))
			}

			mapKeyLower := strings.ToLower(mapKey)
			nv := rv.MapIndex(reflect.ValueOf(mapKeyLower))
			zero := reflect.Value{}
			if nv == zero {
				nv = reflect.New(rt.Elem().Elem())
				if unm, ok := nv.Interface().(json.Unmarshaler); ok {
					// load defaults
					unm.UnmarshalJSON(nil) //nolint:errcheck
				}
				rv.SetMapIndex(reflect.ValueOf(mapKeyLower), nv)
			}

			err := loadEnvInternal(env, prefix+"_"+mapKey, nv.Elem())
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Struct:
		flen := rt.NumField()
		for i := 0; i < flen; i++ {
			f := rt.Field(i)

			// load only public fields
			if f.Tag.Get("json") == "-" {
				continue
			}

			err := loadEnvInternal(env, prefix+"_"+strings.ToUpper(f.Name), rv.Field(i))
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Slice:
		if rt.Elem() == reflect.TypeOf("") {
			if ev, ok := env[prefix]; ok {
				if ev == "" {
					rv.Set(reflect.MakeSlice(rv.Type(), 0, 0))
				} else {
					rv.Set(reflect.ValueOf(strings.Split(ev, ",")))
				}
			}
			return nil
		}

		if rt.Elem().Kind() == reflect.Struct {
			if ev, ok := env[prefix]; ok && ev == "" { // special case: empty list
				rv.Set(reflect.MakeSlice(rv.Type(), 0, 0))
			} else {
				for i := 0; ; i++ {
					itemPrefix := prefix + "_" + strconv.FormatInt(int64(i), 10)
					if !envHasAtLeastAKeyWithPrefix(env, itemPrefix) {
						break
					}

					elem := reflect.New(rt.Elem())
					err := loadEnvInternal(env, itemPrefix, elem.Elem())
					if err != nil {
						return err
					}

					rv.Set(reflect.Append(rv, elem.Elem()))
				}
			}
			return nil
		}
	}

	return fmt.Errorf("unsupported type: %v", rt)
}

// Load loads the configuration from the environment.
func Load(prefix string, v interface{}) error {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		tmp := strings.SplitN(kv, "=", 2)
		env[tmp[0]] = tmp[1]
	}

	return loadEnvInternal(env, prefix, reflect.ValueOf(v).Elem())
}
