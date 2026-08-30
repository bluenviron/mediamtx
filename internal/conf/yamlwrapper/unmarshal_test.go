package yamlwrapper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf/yamlwrapper"
)

func TestUnmarshalIntegerMapKey(t *testing.T) {
	buf := []byte(`
1: value
test: value2
`)

	var dest any
	err := yamlwrapper.Unmarshal(buf, &dest)
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

	err := yamlwrapper.Unmarshal(buf, &map[string]string{})
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
	err := yamlwrapper.Unmarshal(input, &result)
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
	err := yamlwrapper.Unmarshal(input, &result)
	require.NoError(t, err)
	require.Equal(t, true, result.Field1)
}

func TestUnmarshalDirective(t *testing.T) {
	t.Run("mapping", func(t *testing.T) {
		input := []byte("%YAML 1.1\n" +
			"---\n" +
			"# --- inside a comment\n" +
			"field: yes\n")

		var result struct {
			Field bool `json:"field"`
		}
		err := yamlwrapper.Unmarshal(input, &result)
		require.NoError(t, err)
		require.True(t, result.Field)
	})

	t.Run("version 1.2", func(t *testing.T) {
		input := []byte("%YAML 1.2\n" +
			"---\n" +
			"field: value\n")

		var result map[string]string
		err := yamlwrapper.Unmarshal(input, &result)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"field": "value"}, result)
	})

	t.Run("document marker in literal", func(t *testing.T) {
		input := []byte("%YAML 1.1\n" +
			"---\n" +
			"field: |\n" +
			"  ---\n")

		var result map[string]string
		err := yamlwrapper.Unmarshal(input, &result)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"field": "---\n"}, result)
	})

	t.Run("empty document", func(t *testing.T) {
		input := []byte("%YAML 1.1\n" +
			"---\n")

		var result any
		err := yamlwrapper.Unmarshal(input, &result)
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("duplicate key", func(t *testing.T) {
		input := []byte("%YAML 1.1\n" +
			"---\n" +
			"key: value1\n" +
			"key: value2\n")

		err := yamlwrapper.Unmarshal(input, &map[string]string{})
		require.ErrorContains(t, err, "mapping key \"key\" already defined")
	})

	t.Run("unknown field", func(t *testing.T) {
		input := []byte("%YAML 1.1\n" +
			"---\n" +
			"unknown: value\n")

		var result struct{}
		err := yamlwrapper.Unmarshal(input, &result)
		require.EqualError(t, err, "json: unknown field \"unknown\"")
	})
}

func TestUnmarshalMultipleDocuments(t *testing.T) {
	for _, ca := range []struct {
		name  string
		input string
	}{
		{
			name:  "without directive",
			input: "field: one\n---\nfield: two\n",
		},
		{
			name:  "with directive",
			input: "%YAML 1.1\n---\nfield: one\n---\nfield: two\n",
		},
		{
			name:  "empty before document",
			input: "%YAML 1.1\n---\n# empty\n---\nfield: two\n",
		},
		{
			name:  "empty after document",
			input: "%YAML 1.1\n---\nfield: one\n---\n",
		},
		{
			name:  "empty before document without directive",
			input: "---\n---\nfield: two\n",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			err := yamlwrapper.Unmarshal([]byte(ca.input), &map[string]string{})
			require.EqualError(t, err, "invalid YAML")
		})
	}
}

func TestUnmarshalEmpty(t *testing.T) {
	input := []byte(``)

	var result any
	err := yamlwrapper.Unmarshal(input, &result)
	require.NoError(t, err)
}

func FuzzUnmarshal(f *testing.F) {
	f.Fuzz(func(_ *testing.T, buf []byte) {
		var dest any
		yamlwrapper.Unmarshal(buf, &dest) //nolint:errcheck
	})
}
