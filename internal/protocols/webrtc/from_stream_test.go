package webrtc

import (
	"fmt"
	"testing"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	strm := &stream.Stream{
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Desc: &description.Session{Medias: []*description.Media{{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.MJPEG{}},
		}}},
		GenerateRTPPackets: true,
		Parent:             test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	l := test.Logger(func(logger.Level, string, ...interface{}) {
		t.Error("should not happen")
	})

	err = FromStream(strm, l, nil)
	require.Equal(t, errNoSupportedCodecsFrom, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	strm := &stream.Stream{
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Desc: &description.Session{Medias: []*description.Media{
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{}},
			},
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.MJPEG{}},
			},
		}},
		GenerateRTPPackets: true,
		Parent:             test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	n := 0

	l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
		require.Equal(t, logger.Warn, l)
		if n == 0 {
			require.Equal(t, "skipping track 2 (M-JPEG)", fmt.Sprintf(format, args...))
		}
		n++
	})

	pc := &PeerConnection{}

	err = FromStream(strm, l, pc)
	require.NoError(t, err)
	defer strm.RemoveReader(l)

	require.Equal(t, 1, n)
}

func TestFromStream(t *testing.T) {
	for _, ca := range toFromStreamCases {
		t.Run(ca.name, func(t *testing.T) {
			strm := &stream.Stream{
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Desc: &description.Session{
					Medias: []*description.Media{{
						Formats: []format.Format{ca.in},
					}},
				},
				GenerateRTPPackets: false,
				Parent:             test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			pc := &PeerConnection{}

			err = FromStream(strm, nil, pc)
			require.NoError(t, err)
			defer strm.RemoveReader(nil)

			require.Equal(t, ca.webrtcCaps, pc.OutgoingTracks[0].Caps)
		})
	}
}
