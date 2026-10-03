package stream

import (
	"bytes"
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
	"github.com/stretchr/testify/require"
)

// The parameter sets advertised in the offline description
// (mediasFromAlwaysAvailableTracks) are hand-maintained copies of the ones
// inside the embedded offline_*.mp4 assets. Nothing in the build enforces the
// match, and a mismatch fails softly: format_updater rewrites the description
// from the first in-band access unit, so a stale constant survives a smoke test
// and shows up later as a corrupt first frame on a freshly attached reader.
//
// Re-encoding an asset changes these bytes in content and in length, so pin
// them. Regenerate with scripts/gen-offline-assets.sh in the mnet monorepo.
func TestOfflineParameterSetsMatchAssets(t *testing.T) {
	var h264 pmp4.Presentation
	require.NoError(t, h264.Unmarshal(bytes.NewReader(offlineH264)))
	c264, ok := h264.Tracks[0].Codec.(*codecs.H264)
	require.True(t, ok, "offline_h264.mp4 track 0 is not H264")
	require.Equal(t, c264.SPS, offlineH264SPS)
	require.Equal(t, c264.PPS, offlineH264PPS)

	var h265 pmp4.Presentation
	require.NoError(t, h265.Unmarshal(bytes.NewReader(offlineH265)))
	c265, ok := h265.Tracks[0].Codec.(*codecs.H265)
	require.True(t, ok, "offline_h265.mp4 track 0 is not H265")
	require.Equal(t, c265.VPS, offlineH265VPS)
	require.Equal(t, c265.SPS, offlineH265SPS)
	require.Equal(t, c265.PPS, offlineH265PPS)
}

// The AV1 and VP9 assets carry no parameter-set constants -- their
// configuration is in-band -- but they still have to survive the same
// pmp4.Unmarshal + track-type path that runFile() takes at runtime.
func TestOfflineAssetsDecode(t *testing.T) {
	for _, ca := range []struct {
		name  string
		asset []byte
		codec any
	}{
		{"av1", offlineAV1, &codecs.AV1{}},
		{"vp9", offlineVP9, &codecs.VP9{}},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var pres pmp4.Presentation
			require.NoError(t, pres.Unmarshal(bytes.NewReader(ca.asset)))
			require.IsType(t, ca.codec, pres.Tracks[0].Codec)
			require.NotEmpty(t, pres.Tracks[0].Samples)
		})
	}
}
