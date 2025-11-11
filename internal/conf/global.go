package conf

import (
	"encoding/json"
	"reflect"
)

var globalValuesType = func() reflect.Type {
	var fields []reflect.StructField
	rt := reflect.TypeOf(Conf{})
	nf := rt.NumField()

	for i := range nf {
		f := rt.Field(i)
		j := f.Tag.Get("json")

		if j != "-" && j != "pathDefaults" && j != "paths" {
			fields = append(fields, reflect.StructField{
				Name: f.Name,
				Type: f.Type,
				Tag:  f.Tag,
			})
		}
	}

	return reflect.StructOf(fields)
}()

func newGlobalValues() any {
	return reflect.New(globalValuesType).Interface()
}

// Global is the global part of Conf.
type Global struct {
	Values any
}

// MarshalJSON implements json.Marshaler.
func (p *Global) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Values)
}
