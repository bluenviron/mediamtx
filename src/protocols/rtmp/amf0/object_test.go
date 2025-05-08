package amf0

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObjectGet(t *testing.T) {
	o := Object{{Key: "testme", Value: "ok"}}
	v, ok := o.Get("testme")
	require.Equal(t, true, ok)
	require.Equal(t, "ok", v)
}

func TestObjectGetString(t *testing.T) {
	o := Object{{Key: "testme", Value: "ok"}}
	v, ok := o.GetString("testme")
	require.Equal(t, true, ok)
	require.Equal(t, "ok", v)
}

func TestObjectGetFloat64(t *testing.T) {
	o := Object{{Key: "testme", Value: float64(123)}}
	v, ok := o.GetFloat64("testme")
	require.Equal(t, true, ok)
	require.Equal(t, float64(123), v)
}
