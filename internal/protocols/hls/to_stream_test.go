package hls

import (
	"testing"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/stretchr/testify/require"
)

func TestToStreamNoSupportedCodecs(t *testing.T) {
	_, err := ToStream(nil, []*gohlslib.Track{}, nil)
	require.Equal(t, ErrNoSupportedCodecs, err)
}

// this is impossible to test since currently we support all gohlslib.Tracks.
// func TestToStreamSkipUnsupportedTracks(t *testing.T)
