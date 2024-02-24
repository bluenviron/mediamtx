package amf0

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshal(t *testing.T) {
	for _, ca := range cases {
		t.Run(ca.name, func(t *testing.T) {
			enc, err := Marshal(ca.dec)
			require.NoError(t, err)
			require.Equal(t, ca.enc, enc)
		})
	}
}
