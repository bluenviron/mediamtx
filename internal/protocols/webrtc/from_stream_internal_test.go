package webrtc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestH264IsSEIOnly(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x20}
	pps := []byte{0x68, 0xee, 0x3c, 0x80}
	sei := []byte{0x06, 0x01, 0x02}
	sei2 := []byte{0x06, 0x03, 0x04}
	aud := []byte{0x09, 0xf0}
	idr := []byte{0x65, 0x01}
	nonIDR := []byte{0x01, 0xaa}

	for _, ca := range []struct {
		name     string
		au       unit.PayloadH264
		expected bool
	}{
		{"sei only", unit.PayloadH264{sei}, true},
		{"multiple sei", unit.PayloadH264{sei, sei2}, true},
		{"aud only", unit.PayloadH264{aud}, false},
		{"idr", unit.PayloadH264{sps, pps, idr}, false},
		{"non-idr", unit.PayloadH264{nonIDR}, false},
		{"sei then idr", unit.PayloadH264{sei, idr}, false},
	} {
		t.Run(ca.name, func(t *testing.T) {
			require.Equal(t, ca.expected, h264IsSEIOnly(ca.au))
		})
	}
}
