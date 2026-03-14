package push

import (
	"net/url"
	"testing"

	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"
)

func TestRTMPHostCandidates(t *testing.T) {
	for _, ca := range []struct {
		name     string
		rawURL   string
		expected []string
	}{
		{
			name:     "rtmp default port",
			rawURL:   "rtmp://example.com/app/stream",
			expected: []string{"example.com:1935"},
		},
		{
			name:     "rtmps fallback ports",
			rawURL:   "rtmps://example.com/app/stream",
			expected: []string{"example.com:443", "example.com:1936"},
		},
		{
			name:     "explicit port preserved",
			rawURL:   "rtmps://example.com:8443/app/stream",
			expected: []string{"example.com:8443"},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			u, err := url.Parse(ca.rawURL)
			require.NoError(t, err)
			require.Equal(t, ca.expected, rtmpHostCandidates(u))
		})
	}
}

func TestRTMPTrackParametersChanged(t *testing.T) {
	t.Run("h264", func(t *testing.T) {
		forma := &format.H264{SPS: []byte{1}, PPS: []byte{2}}
		require.False(t, h264TrackParametersChanged(forma, &codecs.H264{SPS: []byte{1}, PPS: []byte{2}}))
		require.True(t, h264TrackParametersChanged(forma, &codecs.H264{SPS: []byte{9}, PPS: []byte{2}}))
	})

	t.Run("h265", func(t *testing.T) {
		forma := &format.H265{VPS: []byte{1}, SPS: []byte{2}, PPS: []byte{3}}
		require.False(t, h265TrackParametersChanged(forma, &codecs.H265{VPS: []byte{1}, SPS: []byte{2}, PPS: []byte{3}}))
		require.True(t, h265TrackParametersChanged(forma, &codecs.H265{VPS: []byte{1}, SPS: []byte{7}, PPS: []byte{3}}))
	})

	t.Run("mpeg4audio", func(t *testing.T) {
		config := &mpeg4audio.AudioSpecificConfig{Type: 2, SampleRate: 48000, ChannelConfig: 2}
		forma := &format.MPEG4Audio{Config: config}
		require.False(t, mpeg4AudioTrackParametersChanged(forma, &codecs.MPEG4Audio{Config: config}))
		require.True(t, mpeg4AudioTrackParametersChanged(forma, &codecs.MPEG4Audio{Config: &mpeg4audio.AudioSpecificConfig{Type: 2, SampleRate: 44100, ChannelConfig: 2}}))
	})
}
