package webrtc

import (
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestFromStream(t *testing.T) {
	for _, ca := range toFromStreamCases {
		if ca.in == nil {
			continue
		}
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{
				Medias: []*description.Media{{
					Formats: []format.Format{ca.in},
				}},
			}

			stream, err := stream.New(
				1460,
				desc,
				false,
				test.NilLogger,
			)
			require.NoError(t, err)
			defer stream.Close()

			writer := asyncwriter.New(0, nil)

			pc := &PeerConnection{}

			err = FromStream(stream, writer, pc)
			require.NoError(t, err)

			require.Equal(t, ca.webrtcCaps, pc.OutgoingTracks[0].Caps)
		})
	}
}
