// Package env contains a function to load configuration from environment.
package env

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Unmarshaler can be implemented to override the unmarshaling process.
type Unmarshaler interface {
	UnmarshalEnv(prefix string, v string) error
}

func envHasAtLeastAKeyWithPrefix(env map[string]string, prefix string) bool {
	for key := range env {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func loadEnvInternal(env map[string]string, prefix string, prv reflect.Value) error {
	if prv.Kind() != reflect.Pointer {
		return loadEnvInternal(env, prefix, prv.Addr())
	}

	rt := prv.Type().Elem()

	if i, ok := prv.Interface().(Unmarshaler); ok {
		if ev, ok := env[prefix]; ok {
			if prv.IsNil() {
				prv.Set(reflect.New(rt))
				i = prv.Interface().(Unmarshaler)
			}
			err := i.UnmarshalEnv(prefix, ev)
			if err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
		} else if envHasAtLeastAKeyWithPrefix(env, prefix) {
			err := i.UnmarshalEnv(prefix, "")
			if err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
		}
		return nil
	}

	switch rt {
	case reflect.TypeOf(""):
		if ev, ok := env[prefix]; ok {
			if prv.IsNil() {
				prv.Set(reflect.New(rt))
			}
			prv.Elem().SetString(ev)
		}
		return nil

	case reflect.TypeOf(int(0)):
		if ev, ok := env[prefix]; ok {
			if prv.IsNil() {
				prv.Set(reflect.New(rt))
			}
			iv, err := strconv.ParseInt(ev, 10, 32)
			if err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
			prv.Elem().SetInt(iv)
		}
		return nil

	case reflect.TypeOf(uint(0)):
		if ev, ok := env[prefix]; ok {
			if prv.IsNil() {
				prv.Set(reflect.New(rt))
			}
			iv, err := strconv.ParseUint(ev, 10, 32)
			if err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
			prv.Elem().SetUint(iv)
		}
		return nil

	case reflect.TypeOf(float64(0)):
		if ev, ok := env[prefix]; ok {
			if prv.IsNil() {
				prv.Set(reflect.New(rt))
			}
			iv, err := strconv.ParseFloat(ev, 64)
			if err != nil {
				return fmt.Errorf("%s: %w", prefix, err)
			}
			prv.Elem().SetFloat(iv)
		}
		return nil

	case reflect.TypeOf(bool(false)):
		if ev, ok := env[prefix]; ok {
			if prv.IsNil() {
				prv.Set(reflect.New(rt))
			}
			switch strings.ToLower(ev) {
			case "yes", "true":
				prv.Elem().SetBool(true)

			case "no", "false":
				prv.Elem().SetBool(false)

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
			if prv.Elem().IsNil() {
				prv.Elem().Set(reflect.MakeMap(rt))
			}

			mapKeyLower := strings.ToLower(mapKey)
			nv := prv.Elem().MapIndex(reflect.ValueOf(mapKeyLower))
			zero := reflect.Value{}
			if nv == zero {
				nv = reflect.New(rt.Elem().Elem())
				prv.Elem().SetMapIndex(reflect.ValueOf(mapKeyLower), nv)
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
			jsonTag := f.Tag.Get("json")

			// load only public fields
			if jsonTag == "-" {
				continue
			}

			err := loadEnvInternal(env, prefix+"_"+
				strings.ToUpper(strings.TrimSuffix(jsonTag, ",omitempty")), prv.Elem().Field(i))
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Slice:
		switch {
		case rt.Elem() == reflect.TypeOf(""):
			if ev, ok := env[prefix]; ok {
				if ev == "" {
					prv.Elem().Set(reflect.MakeSlice(prv.Elem().Type(), 0, 0))
				} else {
					if prv.IsNil() {
						prv.Set(reflect.New(rt))
					}
					prv.Elem().Set(reflect.ValueOf(strings.Split(ev, ",")))
				}
			}
			return nil

		case rt.Elem() == reflect.TypeOf(float64(0)):
			if ev, ok := env[prefix]; ok {
				if ev == "" {
					prv.Elem().Set(reflect.MakeSlice(prv.Elem().Type(), 0, 0))
				} else {
					if prv.IsNil() {
						prv.Set(reflect.New(rt))
					}

					raw := strings.Split(ev, ",")
					vals := make([]float64, len(raw))

					for i, v := range raw {
						tmp, err := strconv.ParseFloat(v, 64)
						if err != nil {
							return err
						}
						vals[i] = tmp
					}

					prv.Elem().Set(reflect.ValueOf(vals))
				}
			}
			return nil

		case rt.Elem().Kind() == reflect.Struct:
			if ev, ok := env[prefix]; ok && ev == "" { // special case: empty list
				prv.Elem().Set(reflect.MakeSlice(prv.Elem().Type(), 0, 0))
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

					prv.Elem().Set(reflect.Append(prv.Elem(), elem.Elem()))
				}
			}
			return nil
		}
	}

	return fmt.Errorf("unsupported type: %v", rt)
}

func loadWithEnv(env map[string]string, prefix string, v interface{}) error {
	return loadEnvInternal(env, prefix, reflect.ValueOf(v).Elem())
}

func envToMap() map[string]string {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		tmp := strings.SplitN(kv, "=", 2)
		env[tmp[0]] = tmp[1]
	}
	return env
}

// Load loads the configuration from the environment.
func Load(prefix string, v interface{}) error {
	return loadWithEnv(envToMap(), prefix, v)
}
