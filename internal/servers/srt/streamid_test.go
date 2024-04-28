package srt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamIDUnmarshal(t *testing.T) {
	for _, ca := range []struct {
		name string
		raw  string
		dec  streamID
	}{
		{
			"mediamtx syntax 1",
			"read:mypath",
			streamID{
				mode: streamIDModeRead,
				path: "mypath",
			},
		},
		{
			"mediamtx syntax 2",
			"publish:mypath:myquery",
			streamID{
				mode:  streamIDModePublish,
				path:  "mypath",
				query: "myquery",
			},
		},
		{
			"mediamtx syntax 3",
			"read:mypath:myuser:mypass:myquery",
			streamID{
				mode:  streamIDModeRead,
				path:  "mypath",
				user:  "myuser",
				pass:  "mypass",
				query: "myquery",
			},
		},
		{
			"standard syntax",
			"#!::u=johnny,t=file,m=publish,r=results.csv,s=mypass,h=myhost.com",
			streamID{
				mode: streamIDModePublish,
				path: "results.csv",
				user: "johnny",
				pass: "mypass",
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var sid streamID
			err := sid.unmarshal(ca.raw)
			require.NoError(t, err)
			require.Equal(t, ca.dec, sid)
		})
	}
}
