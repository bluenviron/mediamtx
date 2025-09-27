package webrtc

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.MJPEG{}},
	}}}

	r := &stream.Reader{
		Parent: test.Logger(func(logger.Level, string, ...interface{}) {
			t.Error("should not happen")
		}),
	}

	err := FromStream(desc, r, nil)
	require.Equal(t, errNoSupportedCodecsFrom, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.MJPEG{}},
		},
	}}

	n := 0

	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...interface{}) {
			require.Equal(t, logger.Warn, l)
			if n == 0 {
				require.Equal(t, "skipping track 2 (M-JPEG)", fmt.Sprintf(format, args...))
			}
			n++
		}),
	}

	pc := &PeerConnection{}

	err := FromStream(desc, r, pc)
	require.NoError(t, err)

	require.Equal(t, 1, n)
}

func TestFromStream(t *testing.T) {
	for _, ca := range toFromStreamCases {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{
				Medias: []*description.Media{{
					Formats: []format.Format{ca.in},
				}},
			}

			pc := &PeerConnection{}
			r := &stream.Reader{Parent: test.NilLogger}

			err := FromStream(desc, r, pc)
			require.NoError(t, err)

			require.Equal(t, ca.webrtcCaps, pc.OutgoingTracks[0].Caps)
		})
	}
}
