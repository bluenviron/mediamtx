package defs_test

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/formatlabel"
)

func TestMediasToTracksMPEG4AudioChannelCount(t *testing.T) {
	for _, ca := range []struct {
		name          string
		channelConfig uint8
		channelCount  int
	}{
		{"stereo", 2, 2},
		{"5.1", 6, 6},
		{"7.1", 7, 8},
	} {
		t.Run(ca.name, func(t *testing.T) {
			tracks := defs.MediasToTracks([]*description.Media{{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.AudioSpecificConfig{
						Type:          mpeg4audio.ObjectTypeAACLC,
						SampleRate:    48000,
						ChannelConfig: ca.channelConfig,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				}},
			}})

			require.Equal(t, []defs.APIPathTrack{{
				Codec: formatlabel.MPEG4Audio,
				CodecProps: &defs.APIPathTrackCodecPropsMPEG4Audio{
					SampleRate:   48000,
					ChannelCount: ca.channelCount,
				},
			}}, tracks)
		})
	}
}
