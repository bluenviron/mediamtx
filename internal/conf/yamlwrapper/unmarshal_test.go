package yamlwrapper

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnmarshalIntegerMapKey(t *testing.T) {
	buf := []byte(`
1: value
test: value2
`)

	var dest any
	err := Unmarshal(buf, &dest)
	require.NoError(t, err)

	require.Equal(t, map[string]any{
		"1":    "value",
		"test": "value2",
	}, dest)
}

func TestUnmarshalDuplicateKey(t *testing.T) {
	buf := []byte(`
key: value1
key: value2
`)

	err := Unmarshal(buf, &map[string]string{})
	require.EqualError(t, err, "[3:1] mapping key \"key\" already defined at [2:1]"+
		"\n   2 | key: value1\n>  3 | key: value2\n       ^\n")
}

func TestUnmarshalUnknownFields(t *testing.T) {
	type testStruct struct {
		Field1 string `json:"field1"`
		Field2 int    `json:"field2"`
	}

	input := []byte(`field1: test
unknownField: value
field2: 456`)

	var result testStruct
	err := Unmarshal(input, &result)
	require.Error(t, err)
	require.EqualError(t, err, "json: unknown field \"unknownField\"")
}

func TestUnmarshalLegacyBools(t *testing.T) {
	type testStruct struct {
		Field1 bool   `json:"field1"`
		Field2 string `json:"field2"`
	}

	input := []byte("field1: yes\n" +
		"field2: \"yes\"\n")

	var result testStruct
	err := Unmarshal(input, &result)
	require.NoError(t, err)
	require.Equal(t, true, result.Field1)
}

func TestUnmarshalEmpty(t *testing.T) {
	input := []byte(``)

	var result any
	err := Unmarshal(input, &result)
	require.NoError(t, err)
}

func FuzzUnmarshal(f *testing.F) {
	f.Fuzz(func(_ *testing.T, buf []byte) {
		var dest any
		Unmarshal(buf, &dest) //nolint:errcheck
	})
}
