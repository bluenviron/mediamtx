package whip_test

import (
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/whip"
)

var linkHeaderCases = []struct {
	name string
	enc  []string
	dec  whip.LinkHeader
}{
	{
		"a",
		[]string{
			`<stun:stun.l.google.com:19302>; rel="ice-server"`,
			`<turns:turn.example.com>; rel="ice-server"; username="myuser\"a?2;B"; ` +
				`credential="mypwd"; credential-type="password"`,
		},
		whip.LinkHeader{
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
	{
		"slashes-and-quotes",
		[]string{
			`<turns:turn.example.com>; rel="ice-server"; username="my\\user\"a"; ` +
				`credential="my\\pwd\"b"; credential-type="password"`,
		},
		[]webrtc.ICEServer{
			{
				URLs:       []string{"turns:turn.example.com"},
				Username:   "my\\user\"a",
				Credential: "my\\pwd\"b",
			},
		},
	},
}

func FuzzLinkHeaderUnmarshal(f *testing.F) {
	f.Add(`<stun:stun.l.google.com:19302>; rel="ice-server"`)
	f.Add(linkHeaderCases[1].enc[0])
	f.Add("x")
	f.Add("<x")
	f.Add(`<x>; rel="ice-server"; x`)
	f.Add(`<x>; rel="ice-server"; username=x`)
	f.Add(`<x>; rel="ice-server"; username=""; credential="x"; credential-type="password"`)
	f.Add(`<x>; rel="ice-server"; username="x\n"; credential="x"; credential-type="password"`)
	f.Add(`<x>; rel="ice-server"; username="x`)
	f.Add(`<x>; rel="ice-server"; username="x"; x`)
	f.Add(`<x>; rel="ice-server"; username="x"; credential=x`)
	f.Add(`<x>; rel="ice-server"; username="x"; credential="x\n"`)
	f.Add(`<x>; rel="ice-server"; username="x"; credential="x\`)
	f.Add(`<x>; rel="ice-server"; username="x"; credential="x"`)
	f.Add(`<x>; rel="ice-server"; username="x"; credential="x"; credential-type="token"`)
	f.Add(`<x>; rel="ice-server"; username="x"; credential="x"; credential-type="password"x`)

	f.Fuzz(func(t *testing.T, v string) {
		var lh whip.LinkHeader
		err := lh.Unmarshal([]string{v})
		if err != nil {
			return
		}

		var roundTrip whip.LinkHeader
		err = roundTrip.Unmarshal(lh.Marshal())
		require.NoError(t, err)
		require.Equal(t, lh, roundTrip)
	})
}

func TestLinkHeaderUnmarshal(t *testing.T) {
	for _, ca := range linkHeaderCases {
		t.Run(ca.name, func(t *testing.T) {
			var lh whip.LinkHeader
			err := lh.Unmarshal(ca.enc)
			require.NoError(t, err)
			require.Equal(t, ca.dec, lh)
		})
	}
}

func TestLinkHeaderMarshal(t *testing.T) {
	for _, ca := range linkHeaderCases {
		t.Run(ca.name, func(t *testing.T) {
			enc := ca.dec.Marshal()
			require.Equal(t, ca.enc, enc)
		})
	}
}

func TestLinkHeaderUnmarshalInvalid(t *testing.T) {
	for _, ca := range []struct {
		name string
		enc  []string
	}{
		{
			"invalid escape in username",
			[]string{
				`<turns:turn.example.com>; rel="ice-server"; username="my\nuser"; ` +
					`credential="mypwd"; credential-type="password"`,
			},
		},
		{
			"truncated escape in credential",
			[]string{
				`<turns:turn.example.com>; rel="ice-server"; username="myuser"; ` +
					`credential="mypwd\"; credential-type="password"`,
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var lh whip.LinkHeader
			err := lh.Unmarshal(ca.enc)
			require.Error(t, err)
		})
	}
}
