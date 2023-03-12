package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/gohlslib"
)

// HLSVariant is the hlsVariant parameter.
type HLSVariant gohlslib.MuxerVariant

// supported HLS variants.
const (
	HLSVariantMPEGTS     HLSVariant = HLSVariant(gohlslib.MuxerVariantMPEGTS)
	HLSVariantFMP4       HLSVariant = HLSVariant(gohlslib.MuxerVariantFMP4)
	HLSVariantLowLatency HLSVariant = HLSVariant(gohlslib.MuxerVariantLowLatency)
)

// MarshalJSON implements json.Marshaler.
func (d HLSVariant) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case HLSVariantMPEGTS:
		out = "mpegts"

	case HLSVariantFMP4:
		out = "fmp4"

	default:
		out = "lowLatency"
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *HLSVariant) UnmarshalJSON(b []byte) error {
	var in string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "mpegts":
		*d = HLSVariantMPEGTS

	case "fmp4":
		*d = HLSVariantFMP4

	case "lowLatency":
		*d = HLSVariantLowLatency

	default:
		return fmt.Errorf("invalid hlsVariant value: '%s'", in)
	}

	return nil
}

// unmarshalEnv implements envUnmarshaler.
func (d *HLSVariant) unmarshalEnv(s string) error {
	return d.UnmarshalJSON([]byte(`"` + s + `"`))
}
