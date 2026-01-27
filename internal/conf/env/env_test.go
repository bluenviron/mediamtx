package env

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ptrOf[T any](v T) *T {
	return &v
}

type myDuration time.Duration

func (d *myDuration) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	du, err := time.ParseDuration(in)
	if err != nil {
		return err
	}
	*d = myDuration(du)

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *myDuration) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}

func TestLoadPrimitives(t *testing.T) {
	type subStruct2 struct {
		MyParam int `json:"myParam"`
	}

	type mapEntry struct {
		MyValue  string     `json:"myValue"`
		MyStruct subStruct2 `json:"myStruct"`
	}

	type testStruct struct {
		MyString           string               `json:"myString"`
		MyStringOpt        *string              `json:"myStringOpt"`
		MyInt              int                  `json:"myInt"`
		MyIntOpt           *int                 `json:"myIntOpt"`
		MyUint             uint                 `json:"myUint"`
		MyUintOpt          *uint                `json:"myUintOpt"`
		MyFloat            float64              `json:"myFloat"`
		MyFloatOpt         *float64             `json:"myFloatOpt"`
		MyBool             bool                 `json:"myBool"`
		MyBoolOpt          *bool                `json:"myBoolOpt"`
		MyDuration         myDuration           `json:"myDuration"`
		MyDurationOpt      *myDuration          `json:"myDurationOpt"`
		MyDurationOptUnset *myDuration          `json:"myDurationOptUnset"`
		MyMap              map[string]*mapEntry `json:"myMap"`
		Unset              *bool                `json:"unset"`
	}

	var s testStruct

	t.Setenv("MYPREFIX_MYSTRING", "testcontent")
	t.Setenv("MYPREFIX_MYSTRINGOPT", "testcontent2")
	t.Setenv("MYPREFIX_MYINT", "123")
	t.Setenv("MYPREFIX_MYINTOPT", "456")
	t.Setenv("MYPREFIX_MYUINT", "8910")
	t.Setenv("MYPREFIX_MYUINTOPT", "112313")
	t.Setenv("MYPREFIX_MYFLOAT", "15.2")
	t.Setenv("MYPREFIX_MYFLOATOPT", "16.2")
	t.Setenv("MYPREFIX_MYBOOL", "yes")
	t.Setenv("MYPREFIX_MYBOOLOPT", "false")
	t.Setenv("MYPREFIX_MYDURATION", "22s")
	t.Setenv("MYPREFIX_MYDURATIONOPT", "30s")
	t.Setenv("MYPREFIX_MYMAP_MYKEY", "")
	t.Setenv("MYPREFIX_MYMAP_MYKEY2_MYVALUE", "asd")
	t.Setenv("MYPREFIX_MYMAP_MYKEY2_MYSTRUCT_MYPARAM", "456")

	err := Load("MYPREFIX", &s)
	require.NoError(t, err)

	require.Equal(t, testStruct{
		MyString:      "testcontent",
		MyStringOpt:   ptrOf("testcontent2"),
		MyInt:         123,
		MyIntOpt:      ptrOf(456),
		MyUint:        8910,
		MyUintOpt:     ptrOf(uint(112313)),
		MyFloat:       15.2,
		MyFloatOpt:    ptrOf(16.2),
		MyBool:        true,
		MyBoolOpt:     ptrOf(false),
		MyDuration:    22000000000,
		MyDurationOpt: ptrOf(myDuration(30000000000)),
		MyMap: map[string]*mapEntry{
			"mykey": {
				MyValue: "",
				MyStruct: subStruct2{
					MyParam: 0,
				},
			},
			"mykey2": {
				MyValue: "asd",
				MyStruct: subStruct2{
					MyParam: 456,
				},
			},
		},
	}, s)
}

func TestLoadSlice(t *testing.T) {
	type testStruct struct {
		MySliceFloat          []float64 `json:"mySliceFloat"`
		MySliceString         []string  `json:"mySliceString"`
		MySliceStringEmpty    []string  `json:"mySliceStringEmpty"`
		MySliceStringOpt      *[]string `json:"mySliceStringOpt"`
		MySliceStringOptUnset *[]string `json:"mySliceStringOptUnset"`
	}

	var s testStruct

	t.Setenv("MYPREFIX_MYSLICEFLOAT", "0.5,0.5")
	t.Setenv("MYPREFIX_MYSLICESTRING", "val1,val2")
	t.Setenv("MYPREFIX_MYSLICESTRINGEMPTY", "")
	t.Setenv("MYPREFIX_MYSLICESTRINGOPT", "aa")

	err := Load("MYPREFIX", &s)
	require.NoError(t, err)

	require.Equal(t, testStruct{
		MySliceFloat: []float64{0.5, 0.5},
		MySliceString: []string{
			"val1",
			"val2",
		},
		MySliceStringEmpty: []string{},
		MySliceStringOpt:   &[]string{"aa"},
	}, s)
}

func TestLoadSliceStruct(t *testing.T) {
	type subStruct struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		MyInt2   int    `json:"myInt2"`
	}

	type testStruct struct {
		MySliceSubStruct         []subStruct  `json:"mySliceSubStruct"`
		MySliceSubStructOpt      *[]subStruct `json:"mySliceSubStructOpt"`
		MySliceSubStructOptUnset *[]subStruct `json:"mySliceSubStructOptUnset"`
	}

	var s testStruct

	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_0_URL", "url1")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_0_USERNAME", "user1")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_0_PASSWORD", "pass1")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_1_URL", "url2")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_1_PASSWORD", "pass2")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTOPT_0_PASSWORD", "pwd")

	err := Load("MYPREFIX", &s)
	require.NoError(t, err)

	require.Equal(t, testStruct{
		MySliceSubStruct: []subStruct{
			{
				URL:      "url1",
				Username: "user1",
				Password: "pass1",
			},
			{
				URL:      "url2",
				Username: "",
				Password: "pass2",
			},
		},
		MySliceSubStructOpt: &[]subStruct{
			{
				Password: "pwd",
			},
		},
	}, s)
}

func TestLoadEmptySliceStruct(t *testing.T) {
	type subStruct struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		MyInt2   int    `json:"myInt2"`
	}

	type testStruct struct {
		MySliceSubStructEmpty []subStruct `json:"mySliceSubStructEmpty"`
	}

	var s testStruct

	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTEMPTY", "")

	err := Load("MYPREFIX", &s)
	require.NoError(t, err)

	require.Equal(t, testStruct{
		MySliceSubStructEmpty: []subStruct{},
	}, s)
}

func TestLoadPreloadedSliceStruct(t *testing.T) {
	type subStruct struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		MyInt2   int    `json:"myInt2"`
	}

	type testStruct struct {
		MySliceSubStructPreloaded  []subStruct `json:"mySliceSubStructPreloaded"`
		MySliceSubStructPreloaded2 []subStruct `json:"mySliceSubStructPreloaded2"`
	}

	s := testStruct{
		MySliceSubStructPreloaded: []subStruct{
			{
				URL:      "val1",
				Username: "val2",
			},
		},
		MySliceSubStructPreloaded2: []subStruct{
			{
				URL:      "val3",
				Username: "val4",
			},
		},
	}

	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTPRELOADED_0_URL", "newurl")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTPRELOADED2_1_URL", "newurl2")

	err := Load("MYPREFIX", &s)
	require.NoError(t, err)

	require.Equal(t, testStruct{
		MySliceSubStructPreloaded: []subStruct{
			{
				URL:      "newurl",
				Username: "val2",
			},
		},
		MySliceSubStructPreloaded2: []subStruct{
			{
				URL:      "val3",
				Username: "val4",
			},
			{
				URL: "newurl2",
			},
		},
	}, s)
}
