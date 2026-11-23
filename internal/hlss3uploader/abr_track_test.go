package hlss3uploader

import (
	"testing"

	"github.com/bluenviron/mediamtx/internal/models"
	"github.com/stretchr/testify/require"
)

// A rendition's directory holds the muxer's video and audio files side by side,
// and the path only names the rendition: the track has to come from the file,
// or every audio segment is counted as video.
func TestPopulateABRMetadataSeparatesAudioFromVideo(t *testing.T) {
	const stream = "2ba098e3-22fa-41d1-889a-6055166c464c/720"

	for _, tt := range []struct {
		file  string
		track string
	}{
		{"9f17692e2710_video1_seg7.mp4", "video"},
		{"9f17692e2710_video1_init.mp4", "video"},
		{"9f17692e2710_audio2_seg7.mp4", "audio"},
		{"9f17692e2710_audio2_init.mp4", "audio"},
		{"9f17692e2710_audio_seg7.mp4", "audio"},
		{"seg1.ts", "video"},
		{"video1_seg0.mp4", "video"},
		{"audio1_seg0.mp4", "audio"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			seg := models.LiveHLSSegment{StreamID: stream}
			populateABRMetadata(&seg, tt.file)
			require.Equal(t, tt.track, seg.TrackType)
			require.Equal(t, "720", seg.Rendition)
			require.Equal(t, "2ba098e3-22fa-41d1-889a-6055166c464c", seg.BaseStreamID)
		})
	}
}

// Paths that already name the track keep it.
func TestPopulateABRMetadataKeepsExplicitTrack(t *testing.T) {
	seg := models.LiveHLSSegment{StreamID: "abc/audio/128"}
	populateABRMetadata(&seg, "x_audio1_seg0.mp4")
	require.Equal(t, "audio", seg.TrackType)

	seg = models.LiveHLSSegment{StreamID: "abc/video/720"}
	populateABRMetadata(&seg, "x_video1_seg0.mp4")
	require.Equal(t, "video", seg.TrackType)

	seg = models.LiveHLSSegment{StreamID: "just-a-name"}
	populateABRMetadata(&seg, "x_audio1_seg0.mp4")
	require.Equal(t, "unknown", seg.TrackType)
}
