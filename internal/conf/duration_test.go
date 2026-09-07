package conf_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
)

var casesDuration = []struct {
	name string
	dec  conf.Duration
	enc  string
}{
	{
		"standard",
		conf.Duration(13456 * time.Second),
		`"3h44m16s"`,
	},
	{
		"days",
		conf.Duration(50 * 13456 * time.Second),
		`"7d18h53m20s"`,
	},
	{
		"days negative",
		conf.Duration(-50 * 13456 * time.Second),
		`"-7d18h53m20s"`,
	},
	{
		"days even",
		conf.Duration(7 * 24 * time.Hour),
		`"7d"`,
	},
}

func TestDurationUnmarshal(t *testing.T) {
	for _, ca := range casesDuration {
		t.Run(ca.name, func(t *testing.T) {
			var dec conf.Duration
			err := dec.UnmarshalJSON([]byte(ca.enc))
			require.NoError(t, err)
			require.Equal(t, ca.dec, dec)
		})
	}
}

func TestDurationMarshal(t *testing.T) {
	for _, ca := range casesDuration {
		t.Run(ca.name, func(t *testing.T) {
			enc, err := ca.dec.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, ca.enc, string(enc))
		})
	}
}
