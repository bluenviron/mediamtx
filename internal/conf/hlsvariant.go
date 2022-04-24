package conf

import (
	"encoding/json"
	"fmt"

	"github.com/aler9/rtsp-simple-server/internal/hls"
)

// HLSVariant is the hlsVariant parameter.
type HLSVariant hls.MuxerVariant

// supported HLS variants.
const (
	HLSVariantMPEGTS HLSVariant = HLSVariant(hls.MuxerVariantMPEGTS)
	HLSVariantFMP4   HLSVariant = HLSVariant(hls.MuxerVariantFMP4)
)

// MarshalJSON marshals a HLSVariant into JSON.
func (d HLSVariant) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case HLSVariantMPEGTS:
		out = "mpegts"

	default:
		out = "fmp4"
	}

	return json.Marshal(out)
}

// UnmarshalJSON unmarshals a HLSVariant from JSON.
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

	default:
		return fmt.Errorf("invalid hlsVariant value: '%s'", in)
	}

	return nil
}

func (d *HLSVariant) unmarshalEnv(s string) error {
	return d.UnmarshalJSON([]byte(`"` + s + `"`))
}
