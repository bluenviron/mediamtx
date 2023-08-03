package whip

import (
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

var linkHeaderCases = []struct {
	name string
	enc  []string
	dec  []webrtc.ICEServer
}{
	{
		"a",
		[]string{
			`<stun:stun.l.google.com:19302>; rel="ice-server"`,
			`<turns:turn.example.com>; rel="ice-server"; username="myuser\"a?2;B"; ` +
				`credential="mypwd"; credential-type="password"`,
		},
		[]webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
			{
				URLs:       []string{"turns:turn.example.com"},
				Username:   "myuser\"a?2;B",
				Credential: "mypwd",
			},
		},
	},
}

func TestLinkHeaderUnmarshal(t *testing.T) {
	for _, ca := range linkHeaderCases {
		t.Run(ca.name, func(t *testing.T) {
			dec, err := LinkHeaderUnmarshal(ca.enc)
			require.NoError(t, err)
			require.Equal(t, ca.dec, dec)
		})
	}
}

func TestLinkHeaderMarshal(t *testing.T) {
	for _, ca := range linkHeaderCases {
		t.Run(ca.name, func(t *testing.T) {
			enc := LinkHeaderMarshal(ca.dec)
			require.Equal(t, ca.enc, enc)
		})
	}
}
