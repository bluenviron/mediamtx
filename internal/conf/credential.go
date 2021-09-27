package conf

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var reCredential = regexp.MustCompile(`^[a-zA-Z0-9!\$\(\)\*\+\.;<=>\[\]\^_\-\{\}]+$`)

const credentialSupportedChars = "A-Z,0-9,!,$,(,),*,+,.,;,<,=,>,[,],^,_,-,{,}"

// Credential is a parameter that is used as username or password.
type Credential string

// MarshalJSON marshals a Credential into JSON.
func (d Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(d))
}

// UnmarshalJSON unmarshals a Credential from JSON.
func (d *Credential) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	if in != "" &&
		!strings.HasPrefix(in, "sha256:") &&
		!reCredential.MatchString(in) {
		return fmt.Errorf("contains unsupported characters (supported are %s)", credentialSupportedChars)
	}

	*d = Credential(in)
	return nil
}
