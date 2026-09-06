package webrtc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestH264HasVCL(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x20}
	pps := []byte{0x68, 0xee, 0x3c, 0x80}
	sei := []byte{0x06, 0x01, 0x02}
	aud := []byte{0x09, 0xf0}
	idr := []byte{0x65, 0x01}
	nonIDR := []byte{0x01, 0xaa}

	for _, ca := range []struct {
		name     string
		au       unit.PayloadH264
		expected bool
	}{
		{"sei only", unit.PayloadH264{sei}, false},
		{"aud only", unit.PayloadH264{aud}, false},
		{"sps pps aud", unit.PayloadH264{sps, pps, aud}, false},
		{"idr", unit.PayloadH264{sps, pps, idr}, true},
		{"non-idr", unit.PayloadH264{nonIDR}, true},
		{"sei then idr", unit.PayloadH264{sei, idr}, true},
	} {
		t.Run(ca.name, func(t *testing.T) {
			require.Equal(t, ca.expected, h264HasVCL(ca.au))
		})
	}
}
