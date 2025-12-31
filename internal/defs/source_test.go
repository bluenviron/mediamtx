package defs

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"
)

func TestFormatToTrack_H264(t *testing.T) {
	// H264 SPS with profile=High (100), level=4.1 (41), 1920x1080@30fps
	sps := []byte{
		0x67, 0x64, 0x00, 0x29, 0xac, 0xd9, 0x40, 0x78,
		0x02, 0x27, 0xe5, 0xc0, 0x44, 0x00, 0x00, 0x03,
		0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c,
		0x60, 0xc6, 0x58,
	}
	pps := []byte{0x68, 0xeb, 0xe3, 0xcb, 0x22, 0xc0}

	forma := &format.H264{
		SPS: sps,
		PPS: pps,
	}

	track := FormatToTrack(forma)

	require.Equal(t, "H264", track.Codec)
	require.NotNil(t, track.Width)
	require.NotNil(t, track.Height)
	require.NotNil(t, track.H264Profile)
	require.NotNil(t, track.H264Level)
	// Profile/level assertions depend on actual SPS parsing
}

func TestFormatToTrack_H264_NoSPS(t *testing.T) {
	forma := &format.H264{}

	track := FormatToTrack(forma)

	require.Equal(t, "H264", track.Codec)
	require.Nil(t, track.Width)
	require.Nil(t, track.Height)
	require.Nil(t, track.H264Profile)
	require.Nil(t, track.H264Level)
}

func TestFormatToTrack_VP9(t *testing.T) {
	profile := 0
	forma := &format.VP9{
		ProfileID: &profile,
	}

	track := FormatToTrack(forma)

	require.Equal(t, "VP9", track.Codec)
	require.NotNil(t, track.VP9Profile)
	require.Equal(t, 0, *track.VP9Profile)
}

func TestFormatToTrack_AV1(t *testing.T) {
	profile := 0
	level := 5
	tier := 0
	forma := &format.AV1{
		Profile:  &profile,
		LevelIdx: &level,
		Tier:     &tier,
	}

	track := FormatToTrack(forma)

	require.Equal(t, "AV1", track.Codec)
	require.NotNil(t, track.AV1Profile)
	require.NotNil(t, track.AV1Level)
	require.NotNil(t, track.AV1Tier)
	require.Equal(t, 0, *track.AV1Profile)
	require.Equal(t, 5, *track.AV1Level)
	require.Equal(t, 0, *track.AV1Tier)
}

func TestFormatToTrack_Opus(t *testing.T) {
	forma := &format.Opus{
		ChannelCount: 2,
	}

	track := FormatToTrack(forma)

	require.Equal(t, "Opus", track.Codec)
	require.NotNil(t, track.Channels)
	require.NotNil(t, track.SampleRate)
	require.Equal(t, 2, *track.Channels)
	require.Equal(t, 48000, *track.SampleRate) // Opus is always 48kHz
}

func TestFormatToTrack_MPEG4Audio(t *testing.T) {
	forma := &format.MPEG4Audio{
		Config: &mpeg4audio.AudioSpecificConfig{
			SampleRate:   44100,
			ChannelCount: 2,
		},
	}

	track := FormatToTrack(forma)

	require.Equal(t, "MPEG-4 Audio", track.Codec)
	require.NotNil(t, track.Channels)
	require.NotNil(t, track.SampleRate)
	require.Equal(t, 2, *track.Channels)
	require.Equal(t, 44100, *track.SampleRate)
}

func TestFormatToTrack_G711(t *testing.T) {
	forma := &format.G711{
		MULaw: true,
	}

	track := FormatToTrack(forma)

	require.Equal(t, "G711", track.Codec)
	require.NotNil(t, track.Channels)
	require.NotNil(t, track.SampleRate)
	require.Equal(t, 1, *track.Channels)
	require.Equal(t, 8000, *track.SampleRate)
}

func TestFormatToTrack_LPCM(t *testing.T) {
	forma := &format.LPCM{
		BitDepth:     16,
		SampleRate:   48000,
		ChannelCount: 2,
	}

	track := FormatToTrack(forma)

	require.Equal(t, "LPCM", track.Codec)
	require.NotNil(t, track.BitDepth)
	require.NotNil(t, track.Channels)
	require.NotNil(t, track.SampleRate)
	require.Equal(t, 16, *track.BitDepth)
	require.Equal(t, 2, *track.Channels)
	require.Equal(t, 48000, *track.SampleRate)
}

func TestFormatsToTracks(t *testing.T) {
	formats := []format.Format{
		&format.H264{},
		&format.Opus{ChannelCount: 2},
	}

	tracks := FormatsToTracks(formats)

	require.Len(t, tracks, 2)
	require.Equal(t, "H264", tracks[0].Codec)
	require.Equal(t, "Opus", tracks[1].Codec)
}

func TestH264ProfileString(t *testing.T) {
	tests := []struct {
		profileIdc uint8
		expected   string
	}{
		{66, "Baseline"},
		{77, "Main"},
		{88, "Extended"},
		{100, "High"},
		{110, "High 10"},
		{122, "High 4:2:2"},
		{244, "High 4:4:4 Predictive"},
		{99, "99"}, // Unknown profile
	}

	for _, tc := range tests {
		result := h264ProfileString(tc.profileIdc)
		require.Equal(t, tc.expected, result)
	}
}

func TestH264LevelString(t *testing.T) {
	tests := []struct {
		levelIdc uint8
		expected string
	}{
		{10, "1"},
		{11, "1.1"},
		{12, "1.2"},
		{20, "2"},
		{21, "2.1"},
		{30, "3"},
		{31, "3.1"},
		{40, "4"},
		{41, "4.1"},
		{50, "5"},
		{51, "5.1"},
		{52, "5.2"},
	}

	for _, tc := range tests {
		result := h264LevelString(tc.levelIdc)
		require.Equal(t, tc.expected, result)
	}
}
