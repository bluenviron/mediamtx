package conf

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRTSPSourceMediaTypes(t *testing.T) {
	for _, ca := range []struct {
		values string
		err    string
	}{
		{"[video]", ""},
		{"[audio]", ""},
		{"[video, audio]", ""},
		{"[]", "'rtspSourceMediaTypes' must include video or audio"},
		{"[application]", "invalid 'rtspSourceMediaTypes' entry: application"},
		{"[video, video]", "duplicate 'rtspSourceMediaTypes' entry: video"},
	} {
		t.Run(ca.values, func(t *testing.T) {
			file := createTempFile(t, []byte("paths:\n  camera:\n"+
				"    source: rtsp://127.0.0.1:8555/camera\n    rtspSourceMediaTypes: "+ca.values+"\n"))
			_, _, err := Load(file, nil, nil)
			if ca.err == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, ca.err)
			}
		})
	}
}
