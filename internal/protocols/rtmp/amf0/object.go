package amf0

// ObjectEntry is an entry of Object.
type ObjectEntry struct {
	Key   string
	Value interface{}
}

// Object is an AMF0 object.
type Object []ObjectEntry

// ECMAArray is an AMF0 ECMA Array.
type ECMAArray Object

// StrictArray is an AMF0 Strict Array.
type StrictArray []interface{}

// Get returns the value corresponding to key.
func (o Object) Get(key string) (interface{}, bool) {
	for _, item := range o {
		if item.Key == key {
			return item.Value, true
		}
	}
	return nil, false
}

// GetString returns the value corresponding to key, only if that is a string.
func (o Object) GetString(key string) (string, bool) {
	v, ok := o.Get(key)
	if !ok {
		return "", false
	}

	v2, ok2 := v.(string)
	if !ok2 {
		return "", false
	}

	return v2, ok2
}

// GetFloat64 returns the value corresponding to key, only if that is a float64.
func (o Object) GetFloat64(key string) (float64, bool) {
	v, ok := o.Get(key)
	if !ok {
		return 0, false
	}

	v2, ok2 := v.(float64)
	if !ok2 {
		return 0, false
	}

	return v2, ok2
}
