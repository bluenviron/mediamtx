package conf

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var reCredential = regexp.MustCompile(`^[a-zA-Z0-9!\$\(\)\*\+\.;<=>\[\]\^_\-\{\}@#&]+$`)

const credentialSupportedChars = "A-Z,0-9,!,$,(,),*,+,.,;,<,=,>,[,],^,_,-,\",\",@,#,&"

// Credential is a parameter that is used as username or password.
type Credential string

// MarshalJSON implements json.Marshaler.
func (d Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(d))
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Credential) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	if in != "" &&
		!strings.HasPrefix(in, "sha256:") &&
		!reCredential.MatchString(in) {
		return fmt.Errorf("credential contains unsupported characters. Supported are: %s", credentialSupportedChars)
	}

	*d = Credential(in)
	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *Credential) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
