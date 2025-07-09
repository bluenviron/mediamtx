package rtmp

import (
	"bytes"
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
			Formats: []format.Format{&format.VP8{}},
		}}},
		GenerateRTPPackets: true,
		Parent:             test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)

	l := test.Logger(func(logger.Level, string, ...interface{}) {
		t.Error("should not happen")
	})

	err = FromStream(strm, l, nil, nil, 0)
	require.Equal(t, errNoSupportedCodecsFrom, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	strm := &stream.Stream{
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Desc: &description.Session{Medias: []*description.Media{
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.VP8{}},
			},
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{}},
			},
			{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{}},
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
		switch n {
		case 0:
			require.Equal(t, "skipping track 1 (VP8)", fmt.Sprintf(format, args...))
		case 1:
			require.Equal(t, "skipping track 3 (H264)", fmt.Sprintf(format, args...))
		}
		n++
	})

	var buf bytes.Buffer
	c := &dummyConn{
		rw: &buf,
	}
	c.initialize()

	err = FromStream(strm, l, c, nil, 0)
	require.NoError(t, err)
	defer strm.RemoveReader(l)

	require.Equal(t, 2, n)
}
