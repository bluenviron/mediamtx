package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RecordFormat is the recordFormat parameter.
type RecordFormat string

// supported values.
const (
	RecordFormatFMP4   RecordFormat = "fmp4"
	RecordFormatMPEGTS RecordFormat = "mpegts"
)

// UnmarshalJSON implements json.Unmarshaler.
func (d *RecordFormat) UnmarshalJSON(b []byte) error {
	type alias RecordFormat
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	switch *d {
	case RecordFormatFMP4, RecordFormatMPEGTS:

	default:
		return fmt.Errorf("invalid record format '%s'", *d)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RecordFormat) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
