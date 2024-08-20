package env

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func stringPtr(v string) *string {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func uintPtr(v uint) *uint {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func float64Ptr(v float64) *float64 {
	return &v
}

func durationPtr(v time.Duration) *time.Duration {
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
	MyString                 string               `json:"myString"`
	MyStringOpt              *string              `json:"myStringOpt"`
	MyInt                    int                  `json:"myInt"`
	MyIntOpt                 *int                 `json:"myIntOpt"`
	MyUint                   uint                 `json:"myUint"`
	MyUintOpt                *uint                `json:"myUintOpt"`
	MyFloat                  float64              `json:"myFloat"`
	MyFloatOpt               *float64             `json:"myFloatOpt"`
	MyBool                   bool                 `json:"myBool"`
	MyBoolOpt                *bool                `json:"myBoolOpt"`
	MyDuration               myDuration           `json:"myDuration"`
	MyDurationOpt            *myDuration          `json:"myDurationOpt"`
	MyDurationOptUnset       *myDuration          `json:"myDurationOptUnset"`
	MyMap                    map[string]*mapEntry `json:"myMap"`
	MySliceFloat             []float64            `json:"mySliceFloat"`
	MySliceString            []string             `json:"mySliceString"`
	MySliceStringEmpty       []string             `json:"mySliceStringEmpty"`
	MySliceStringOpt         *[]string            `json:"mySliceStringOpt"`
	MySliceStringOptUnset    *[]string            `json:"mySliceStringOptUnset"`
	MySliceSubStruct         []mySubStruct        `json:"mySliceSubStruct"`
	MySliceSubStructEmpty    []mySubStruct        `json:"mySliceSubStructEmpty"`
	MySliceSubStructOpt      *[]mySubStruct       `json:"mySliceSubStructOpt"`
	MySliceSubStructOptUnset *[]mySubStruct       `json:"mySliceSubStructOptUnset"`
	Unset                    *bool                `json:"unset"`
}

func TestLoad(t *testing.T) {
	env := map[string]string{
		"MYPREFIX_MYSTRING":                       "testcontent",
		"MYPREFIX_MYSTRINGOPT":                    "testcontent2",
		"MYPREFIX_MYINT":                          "123",
		"MYPREFIX_MYINTOPT":                       "456",
		"MYPREFIX_MYUINT":                         "8910",
		"MYPREFIX_MYUINTOPT":                      "112313",
		"MYPREFIX_MYFLOAT":                        "15.2",
		"MYPREFIX_MYFLOATOPT":                     "16.2",
		"MYPREFIX_MYBOOL":                         "yes",
		"MYPREFIX_MYBOOLOPT":                      "false",
		"MYPREFIX_MYDURATION":                     "22s",
		"MYPREFIX_MYDURATIONOPT":                  "30s",
		"MYPREFIX_MYMAP_MYKEY":                    "",
		"MYPREFIX_MYMAP_MYKEY2_MYVALUE":           "asd",
		"MYPREFIX_MYMAP_MYKEY2_MYSTRUCT_MYPARAM":  "456",
		"MYPREFIX_MYSLICEFLOAT":                   "0.5,0.5",
		"MYPREFIX_MYSLICESTRING":                  "val1,val2",
		"MYPREFIX_MYSLICESTRINGEMPTY":             "",
		"MYPREFIX_MYSLICESTRINGOPT":               "aa",
		"MYPREFIX_MYSLICESUBSTRUCT_0_URL":         "url1",
		"MYPREFIX_MYSLICESUBSTRUCT_0_USERNAME":    "user1",
		"MYPREFIX_MYSLICESUBSTRUCT_0_PASSWORD":    "pass1",
		"MYPREFIX_MYSLICESUBSTRUCT_1_URL":         "url2",
		"MYPREFIX_MYSLICESUBSTRUCT_1_PASSWORD":    "pass2",
		"MYPREFIX_MYSLICESUBSTRUCTEMPTY":          "",
		"MYPREFIX_MYSLICESUBSTRUCTOPT_1_PASSWORD": "pwd",
	}

	for key, val := range env {
		t.Setenv(key, val)
	}

	var s testStruct
	err := Load("MYPREFIX", &s)
	require.NoError(t, err)

	require.Equal(t, testStruct{
		MyString:      "testcontent",
		MyStringOpt:   stringPtr("testcontent2"),
		MyInt:         123,
		MyIntOpt:      intPtr(456),
		MyUint:        8910,
		MyUintOpt:     uintPtr(112313),
		MyFloat:       15.2,
		MyFloatOpt:    float64Ptr(16.2),
		MyBool:        true,
		MyBoolOpt:     boolPtr(false),
		MyDuration:    22000000000,
		MyDurationOpt: (*myDuration)(durationPtr(30000000000)),
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
