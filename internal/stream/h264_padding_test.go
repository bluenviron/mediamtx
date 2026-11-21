package stream

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestH264CameraPadding(t *testing.T) {
	var capture struct {
		CavlcPPS    string                           `json:"cavlc_pps"`
		CabacPPS    string                           `json:"cabac_pps"`
		CavlcPadded struct{ Input, Expected string } `json:"cavlc_padded"`
		CavlcClean  string                           `json:"cavlc_clean"`
		CabacPadded string                           `json:"cabac_padded"`
	}
	data, err := os.ReadFile("testdata/h264_camera_padding.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}
	decode := func(value string) []byte {
		result, decodeErr := hex.DecodeString(value)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		return result
	}
	for _, test := range []struct{ name, pps, input, expected string }{
		{"camera CAVLC padding", capture.CavlcPPS, capture.CavlcPadded.Input, capture.CavlcPadded.Expected},
		{"clean CAVLC slice", capture.CavlcPPS, capture.CavlcClean, capture.CavlcClean},
		{"legal CABAC padding", capture.CabacPPS, capture.CabacPadded, capture.CabacPadded},
		{"unknown entropy coding", "", capture.CavlcPadded.Input, capture.CavlcPadded.Input},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := decode(test.input)
			original := bytes.Clone(input)
			out := unitRemuxerH264(&format.H264{PPS: decode(test.pps)}, unit.PayloadH264{input}).(unit.PayloadH264)
			if len(out) != 1 {
				t.Fatalf("expected one output NAL unit, got %d", len(out))
			}
			if !bytes.Equal(out[0], decode(test.expected)) {
				t.Fatalf("unexpected normalized bytes: original=%d output=%d expected=%d",
					len(input), len(out[0]), len(decode(test.expected)))
			}
			if !bytes.Equal(input, original) {
				t.Fatal("normalization mutated the publisher's shared bytes")
			}
		})
	}
}
