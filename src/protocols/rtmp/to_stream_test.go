package rtmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToStreamNoSupportedCodecs(t *testing.T) {
	r := &Reader{}

	_, err := ToStream(r, nil)
	require.Equal(t, errNoSupportedCodecsTo, err)
}

// this is impossible to test since currently we support all RTMP tracks.
// func TestToStreamSkipUnsupportedTracks(t *testing.T)
