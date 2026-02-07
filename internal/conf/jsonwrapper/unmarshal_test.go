package jsonwrapper

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type testStruct struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

func TestUnmarshalDisallowUnknownFields(t *testing.T) {
	input := strings.NewReader(`{"field1": "test", "unknownField": "value", "field2": 456}`)
	var result testStruct
	err := Decode(input, &result)
	require.Error(t, err)
	require.EqualError(t, err, "json: unknown field \"unknownField\"")
}

func TestUnmarshalPreventSliceReuse(t *testing.T) {
	t.Run("a", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		slice := []Person{
			{Name: "John", Age: 30},
			{Name: "Jane", Age: 25},
		}

		json := []byte(`[{"name": "Bob"}]`)
		err := Unmarshal(json, &slice)
		require.NoError(t, err)

		require.Equal(t, []Person{{
			Name: "Bob",
			Age:  0,
		}}, slice)
	})

	t.Run("b", func(t *testing.T) {
		type Config struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
			Port    int    `json:"port"`
		}
		type Settings struct {
			Configs []Config `json:"configs"`
		}

		settings := Settings{
			Configs: []Config{
				{Name: "old1", Enabled: true, Port: 8080},
				{Name: "old2", Enabled: false, Port: 9090},
				{Name: "old3", Enabled: true, Port: 7070},
			},
		}

		json := []byte(`{"configs": [{"name": "new1"}, {"name": "new2"}]}`)
		err := Unmarshal(json, &settings)
		require.NoError(t, err)

		require.Equal(t, Settings{
			Configs: []Config{
				{Name: "new1", Enabled: false, Port: 0},
				{Name: "new2", Enabled: false, Port: 0},
			},
		}, settings)
	})
}

func TestUnmarshalSetSliceToNil(t *testing.T) {
	t.Run("top level", func(t *testing.T) {
		type Data struct {
			Items []string `json:"items"`
		}

		var data Data

		json := []byte(`{"items": null}`)
		err := Unmarshal(json, &data)
		require.EqualError(t, err, "cannot set slice 'items' to nil")

		data = Data{Items: []string{"a", "b"}}

		json = []byte(`{"items": null}`)
		err = Unmarshal(json, &data)
		require.EqualError(t, err, "cannot set slice 'items' to nil")
	})

	t.Run("nested", func(t *testing.T) {
		type Inner struct {
			Values []int `json:"values"`
		}
		type Outer struct {
			Inner Inner `json:"inner"`
		}

		var data Outer
		json := []byte(`{"inner": {"values": null}}`)
		err := Unmarshal(json, &data)
		require.EqualError(t, err, "cannot set slice 'inner.values' to nil")
	})
}

func TestUnmarshalSetNullableSliceToNil(t *testing.T) {
	type Data struct {
		Items *[]string `json:"items"`
	}

	var data Data

	json := []byte(`{"items": null}`)
	err := Unmarshal(json, &data)
	require.NoError(t, err)

	data = Data{Items: &[]string{"a", "b"}}

	json = []byte(`{"items": null}`)
	err = Unmarshal(json, &data)
	require.NoError(t, err)
}
