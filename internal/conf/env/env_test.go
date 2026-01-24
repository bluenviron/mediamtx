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

type subStruct struct {
	MyParam int `json:"myParam"`
}

type mapEntry struct {
	MyValue  string    `json:"myValue"`
	MyStruct subStruct `json:"myStruct"`
}

type mySubStruct struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	MyInt2   int    `json:"myInt2"`
}

type testStruct struct {
	MyString                   string               `json:"myString"`
	MyStringOpt                *string              `json:"myStringOpt"`
	MyInt                      int                  `json:"myInt"`
	MyIntOpt                   *int                 `json:"myIntOpt"`
	MyUint                     uint                 `json:"myUint"`
	MyUintOpt                  *uint                `json:"myUintOpt"`
	MyFloat                    float64              `json:"myFloat"`
	MyFloatOpt                 *float64             `json:"myFloatOpt"`
	MyBool                     bool                 `json:"myBool"`
	MyBoolOpt                  *bool                `json:"myBoolOpt"`
	MyDuration                 myDuration           `json:"myDuration"`
	MyDurationOpt              *myDuration          `json:"myDurationOpt"`
	MyDurationOptUnset         *myDuration          `json:"myDurationOptUnset"`
	MyMap                      map[string]*mapEntry `json:"myMap"`
	MySliceFloat               []float64            `json:"mySliceFloat"`
	MySliceString              []string             `json:"mySliceString"`
	MySliceStringEmpty         []string             `json:"mySliceStringEmpty"`
	MySliceStringOpt           *[]string            `json:"mySliceStringOpt"`
	MySliceStringOptUnset      *[]string            `json:"mySliceStringOptUnset"`
	MySliceSubStruct           []mySubStruct        `json:"mySliceSubStruct"`
	MySliceSubStructEmpty      []mySubStruct        `json:"mySliceSubStructEmpty"`
	MySliceSubStructOpt        *[]mySubStruct       `json:"mySliceSubStructOpt"`
	MySliceSubStructOptUnset   *[]mySubStruct       `json:"mySliceSubStructOptUnset"`
	MySliceSubStructPreloaded  []mySubStruct        `json:"mySliceSubStructPreloaded"`
	MySliceSubStructPreloaded2 []mySubStruct        `json:"mySliceSubStructPreloaded2"`
	Unset                      *bool                `json:"unset"`
}

func TestLoad(t *testing.T) {
	s := testStruct{
		MySliceSubStructPreloaded: []mySubStruct{
			{
				URL:      "val1",
				Username: "val2",
			},
		},
		MySliceSubStructPreloaded2: []mySubStruct{
			{
				URL:      "val3",
				Username: "val4",
			},
		},
	}

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
	t.Setenv("MYPREFIX_MYSLICEFLOAT", "0.5,0.5")
	t.Setenv("MYPREFIX_MYSLICESTRING", "val1,val2")
	t.Setenv("MYPREFIX_MYSLICESTRINGEMPTY", "")
	t.Setenv("MYPREFIX_MYSLICESTRINGOPT", "aa")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_0_URL", "url1")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_0_USERNAME", "user1")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_0_PASSWORD", "pass1")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_1_URL", "url2")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCT_1_PASSWORD", "pass2")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTEMPTY", "")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTOPT_1_PASSWORD", "pwd")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTPRELOADED_0_URL", "newurl")
	t.Setenv("MYPREFIX_MYSLICESUBSTRUCTPRELOADED2_1_URL", "newurl2")

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
				MyStruct: subStruct{
					MyParam: 0,
				},
			},
			"mykey2": {
				MyValue: "asd",
				MyStruct: subStruct{
					MyParam: 456,
				},
			},
		},
		MySliceFloat: []float64{0.5, 0.5},
		MySliceString: []string{
			"val1",
			"val2",
		},
		MySliceStringEmpty: []string{},
		MySliceStringOpt:   &[]string{"aa"},
		MySliceSubStruct: []mySubStruct{
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
		MySliceSubStructEmpty: []mySubStruct{},
		MySliceSubStructPreloaded: []mySubStruct{
			{
				URL:      "newurl",
				Username: "val2",
			},
		},
		MySliceSubStructPreloaded2: []mySubStruct{
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

func FuzzLoad(f *testing.F) {
	f.Add("MYPREFIX_MYINT", "a")
	f.Add("MYPREFIX_MYUINT", "a")
	f.Add("MYPREFIX_MYFLOAT", "a")
	f.Add("MYPREFIX_MYBOOL", "a")
	f.Add("MYPREFIX_MYSLICESUBSTRUCT_0_MYINT2", "a")
	f.Add("MYPREFIX_MYDURATION", "a")
	f.Add("MYPREFIX_MYDURATION_A", "a")

	f.Fuzz(func(_ *testing.T, key string, val string) {
		env := map[string]string{
			key: val,
		}

		var s testStruct
		loadWithEnv(env, "MYPREFIX", &s) //nolint:errcheck
	})
}
