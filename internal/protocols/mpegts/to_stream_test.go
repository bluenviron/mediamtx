package mpegts

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestToStreamNoSupportedCodecs(t *testing.T) {
	var buf bytes.Buffer
	mux := astits.NewMuxer(context.Background(), &buf)

	err := mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 122,
		StreamType:    astits.StreamTypeDTSAudio,
	})
	require.NoError(t, err)

	mux.SetPCRPID(122)

	_, err = mux.WriteTables()
	require.NoError(t, err)

	r := &mpegts.Reader{R: &buf}
	err = r.Initialize()
	require.NoError(t, err)

	var str *stream.Stream

	// Use a simple logger that just logs the message
	l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
		t.Logf("Log: %s", fmt.Sprintf(format, args...))
	})

	// The function should now return medias with a generic format
	// instead of returning an error
	medias, err := ToStream(r, &str, l)
	require.NoError(t, err)
	require.NotNil(t, medias)
	require.NotEmpty(t, medias)

	// The stream should be initialized with the provided medias
	if str != nil {
		require.Equal(t, medias, str.Desc.Medias)
	}
}

func TestToStreamSkipUnsupportedTracks(t *testing.T) {
	var buf bytes.Buffer
	mux := astits.NewMuxer(context.Background(), &buf)

	err := mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 122,
		StreamType:    astits.StreamTypeDTSAudio,
	})
	require.NoError(t, err)

	err = mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 123,
		StreamType:    astits.StreamTypeH264Video,
	})
	require.NoError(t, err)

	mux.SetPCRPID(122)

	_, err = mux.WriteTables()
	require.NoError(t, err)

	r := &mpegts.Reader{R: &buf}
	err = r.Initialize()
	require.NoError(t, err)

	n := 0

	l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
		require.Equal(t, logger.Warn, l)
		if n == 0 {
			require.Equal(t, "skipping track 1 (unsupported codec)", fmt.Sprintf(format, args...))
		}
		n++
	})

	_, err = ToStream(r, nil, l)
	require.NoError(t, err)
}
