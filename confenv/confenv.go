package confenv

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func process(env map[string]string, envKey string, rv reflect.Value) error {
	rt := rv.Type()

	switch rt {
	case reflect.TypeOf(time.Duration(0)):
		if ev, ok := env[envKey]; ok {
			d, err := time.ParseDuration(ev)
			if err != nil {
				return fmt.Errorf("%s: %s", envKey, err)
			}
			rv.Set(reflect.ValueOf(d))
		}
		return nil
	}

	switch rt.Kind() {
	case reflect.String:
		if ev, ok := env[envKey]; ok {
			rv.SetString(ev)
		}
		return nil

	case reflect.Int:
		if ev, ok := env[envKey]; ok {
			iv, err := strconv.ParseInt(ev, 10, 64)
			if err != nil {
				return fmt.Errorf("%s: %s", envKey, err)
			}
			rv.SetInt(iv)
		}
		return nil

	case reflect.Bool:
		if ev, ok := env[envKey]; ok {
			switch strings.ToLower(ev) {
			case "yes", "true":
				rv.SetBool(true)

			case "no", "false":
				rv.SetBool(false)

			default:
				return fmt.Errorf("%s: invalid value '%s'", envKey, ev)
			}
		}
		return nil

	case reflect.Slice:
		if rt.Elem().Kind() == reflect.String {
			if ev, ok := env[envKey]; ok {
				nv := reflect.Zero(rt)
				for _, sv := range strings.Split(ev, ",") {
					nv = reflect.Append(nv, reflect.ValueOf(sv))
				}
				rv.Set(nv)
			}
			return nil
		}

	case reflect.Map:
		for k := range env {
			if !strings.HasPrefix(k, envKey+"_") {
				continue
			}

			tmp := strings.Split(k[len(envKey+"_"):], "_")
			mapKey := tmp[0]
			if len(mapKey) == 0 {
				continue
			}
			mapKey = strings.ToLower(mapKey)

			// initialize only if there's at least one key
			if rv.IsNil() {
				rv.Set(reflect.MakeMap(rt))
			}

			nv := rv.MapIndex(reflect.ValueOf(mapKey))
			zero := reflect.Value{}
			if nv == zero {
				nv = reflect.New(rt.Elem().Elem())
				rv.SetMapIndex(reflect.ValueOf(mapKey), nv)
			}

			err := process(env, envKey+"_"+strings.ToUpper(mapKey), nv.Elem())
			if err != nil {
				return err
			}
		}
		return nil

	case reflect.Struct:
		flen := rt.NumField()
		for i := 0; i < flen; i++ {
			f := rt.Field(i)

			// process only public fields
			if f.Tag.Get("yaml") == "-" {
				continue
			}

			fieldEnvKey := envKey + "_" + strings.ToUpper(f.Name)
			err := process(env, fieldEnvKey, rv.Field(i))
			if err != nil {
				return err
			}
		}
		return nil
	}

	return fmt.Errorf("unsupported type: %v", rt)
}

func Process(envKey string, v interface{}) error {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		tmp := strings.Split(kv, "=")
		env[tmp[0]] = tmp[1]
	}

	return process(env, envKey, reflect.ValueOf(v).Elem())
}
