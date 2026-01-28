package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// Encryption is the rtspEncryption / rtmpEncryption parameter.
type Encryption string

// values.
const (
	EncryptionNo       Encryption = "no"
	EncryptionOptional Encryption = "optional"
	EncryptionStrict   Encryption = "strict"
)

// UnmarshalJSON implements json.Unmarshaler.
func (d *Encryption) UnmarshalJSON(b []byte) error {
	type alias Encryption
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	switch *d {
	case "false":
		*d = EncryptionNo
	case "true", "yes":
		*d = EncryptionStrict
	}

	switch *d {
	case EncryptionNo, EncryptionOptional, EncryptionStrict:

	default:
		return fmt.Errorf("invalid encryption: '%s'", *d)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *Encryption) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
