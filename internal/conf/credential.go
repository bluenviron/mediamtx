package conf

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/matthewhartstonge/argon2"
)

var (
	rePlainCredential = regexp.MustCompile(`^[a-zA-Z0-9!\$\(\)\*\+\.;<=>\[\]\^_\-\{\}@#&]+$`)
	reBase64          = regexp.MustCompile(`^sha256:[a-zA-Z0-9\+/=]+$`)
)

const plainCredentialSupportedChars = "A-Z,0-9,!,$,(,),*,+,.,;,<,=,>,[,],^,_,-,\",\",@,#,&"

// Credential is a parameter that is used as username or password.
type Credential struct {
	value string
}

// MarshalJSON implements json.Marshaler.
func (d Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.value)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Credential) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = Credential{
		value: in,
	}

	return d.validate()
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *Credential) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}

// GetValue returns the value of the credential.
func (d *Credential) GetValue() string {
	return d.value
}

// IsEmpty returns true if the credential is not configured.
func (d *Credential) IsEmpty() bool {
	return d.value == ""
}

// IsSha256 returns true if the credential is a sha256 hash.
func (d *Credential) IsSha256() bool {
	return d.value != "" && strings.HasPrefix(d.value, "sha256:")
}

// IsArgon2 returns true if the credential is an argon2 hash.
func (d *Credential) IsArgon2() bool {
	return d.value != "" && strings.HasPrefix(d.value, "argon2:")
}

// IsHashed returns true if the credential is a sha256 or argon2 hash.
func (d *Credential) IsHashed() bool {
	return d.IsSha256() || d.IsArgon2()
}

func sha256Base64(in string) string {
	h := sha256.New()
	h.Write([]byte(in))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Check returns true if the given value matches the credential.
func (d *Credential) Check(guess string) bool {
	if d.IsSha256() {
		return d.value[len("sha256:"):] == sha256Base64(guess)
	}
	if d.IsArgon2() {
		// TODO: remove matthewhartstonge/argon2 when this PR gets merged into mainline Go:
		// https://go-review.googlesource.com/c/crypto/+/502515
		ok, err := argon2.VerifyEncoded([]byte(guess), []byte(d.value[len("argon2:"):]))
		return ok && err == nil
	}
	if d.IsEmpty() {
		// when no credential is set, any value is valid
		return true
	}

	return d.value == guess
}

func (d *Credential) validate() error {
	if !d.IsEmpty() {
		switch {
		case d.IsSha256():
			if !reBase64.MatchString(d.value) {
				return fmt.Errorf("credential contains unsupported characters, sha256 hash must be base64 encoded")
			}
		case d.IsArgon2():
			// TODO: remove matthewhartstonge/argon2 when this PR gets merged into mainline Go:
			// https://go-review.googlesource.com/c/crypto/+/502515
			_, err := argon2.Decode([]byte(d.value[len("argon2:"):]))
			if err != nil {
				return fmt.Errorf("invalid argon2 hash: %w", err)
			}
		default:
			if !rePlainCredential.MatchString(d.value) {
				return fmt.Errorf("credential contains unsupported characters. Supported are: %s", plainCredentialSupportedChars)
			}
		}
	}
	return nil
}
