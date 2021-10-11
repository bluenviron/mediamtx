package conf

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type envUnmarshaler interface {
	unmarshalEnv(string) error
}

func loadEnvInternal(env map[string]string, prefix string, rv reflect.Value) error {
	rt := rv.Type()

	if i, ok := rv.Addr().Interface().(envUnmarshaler); ok {
		if ev, ok := env[prefix]; ok {
			err := i.unmarshalEnv(ev)
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
			iv, err := strconv.ParseInt(ev, 10, 64)
			if err != nil {
				return fmt.Errorf("%s: %s", prefix, err)
			}
			rv.SetInt(iv)
		}
		return nil

	case reflect.TypeOf(uint64(0)):
		if ev, ok := env[prefix]; ok {
			iv, err := strconv.ParseUint(ev, 10, 64)
			if err != nil {
				return fmt.Errorf("%s: %s", prefix, err)
			}
			rv.SetUint(iv)
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
	}

	return fmt.Errorf("unsupported type: %v", rt)
}

func loadFromEnvironment(prefix string, v interface{}) error {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		tmp := strings.SplitN(kv, "=", 2)
		env[tmp[0]] = tmp[1]
	}

	return loadEnvInternal(env, prefix, reflect.ValueOf(v).Elem())
}
