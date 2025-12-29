package playback

import (
	"io"
	"os"
	"testing"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func writeBenchInit(f io.WriteSeeker) {
	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Codec: &mcodecs.H264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				},
			},
			{
				ID:        2,
				TimeScale: 90000,
				Codec: &mcodecs.MPEG4Audio{
					Config: mpeg4audio.AudioSpecificConfig{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	err := init.Marshal(f)
	if err != nil {
		panic(err)
	}

	_, err = f.Write([]byte{
		0x00, 0x00, 0x00, 0x10, 'm', 'o', 'o', 'f',
	})
	if err != nil {
		panic(err)
	}
}

func BenchmarkFMP4ReadHeader(b *testing.B) {
	f, err := os.CreateTemp(os.TempDir(), "mediamtx-playback-fmp4-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())

	writeBenchInit(f)
	f.Close()

	for b.Loop() {
		func() {
			f, err = os.Open(f.Name())
			if err != nil {
				panic(err)
			}
			defer f.Close()

			_, _, err = segmentFMP4ReadHeader(f)
			if err != nil {
				panic(err)
			}
		}()
	}
}

func TestSegmentFMP4CanBeConcatenated(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	streamID1 := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	streamID2 := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17}

	baseTracks := []*fmp4.InitTrack{
		{
			ID:        1,
			TimeScale: 90000,
			Codec: &mcodecs.H264{
				SPS: test.FormatH264.SPS,
				PPS: test.FormatH264.PPS,
			},
		},
		{
			ID:        2,
			TimeScale: 48000,
			Codec: &mcodecs.MPEG4Audio{
				Config: mpeg4audio.AudioSpecificConfig{
					Type:         mpeg4audio.ObjectTypeAACLC,
					SampleRate:   48000,
					ChannelCount: 2,
				},
			},
		},
	}

	differentTracks := []*fmp4.InitTrack{
		{
			ID:        1,
			TimeScale: 90000,
			Codec: &mcodecs.H264{
				SPS: test.FormatH264.SPS,
				PPS: test.FormatH264.PPS,
			},
		},
	}

	for _, tt := range []struct {
		name     string
		prevInit *fmp4.Init
		prevEnd  time.Time
		curInit  *fmp4.Init
		curStart time.Time
		want     bool
	}{
		{
			name: "with mtxi - consecutive segments, same stream",
			prevInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID1,
						SegmentNumber: 1,
					},
				},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID1,
						SegmentNumber: 2,
					},
				},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     true,
		},
		{
			name: "with mtxi - non-consecutive segments, same stream",
			prevInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID1,
						SegmentNumber: 1,
					},
				},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID1,
						SegmentNumber: 3,
					},
				},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
		{
			name: "with mtxi - consecutive segments, different streams",
			prevInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID1,
						SegmentNumber: 1,
					},
				},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID2,
						SegmentNumber: 2,
					},
				},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
		{
			name: "prev has mtxi, current does not",
			prevInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID1,
						SegmentNumber: 1,
					},
				},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
		{
			name: "prev does not have mtxi, current has mtxi",
			prevInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks: baseTracks,
				UserData: []amp4.IBox{
					&recordstore.Mtxi{
						StreamID:      streamID1,
						SegmentNumber: 1,
					},
				},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
		{
			name: "legacy mode - same tracks, within time tolerance",
			prevInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime.Add(10 * time.Second),
			curInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(10 * time.Second),
			want:     true,
		},
		{
			name: "legacy mode - same tracks, exactly at tolerance boundary (before)",
			prevInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime.Add(10 * time.Second),
			curInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(9 * time.Second),
			want:     true,
		},
		{
			name: "legacy mode - same tracks, exactly at tolerance boundary (after)",
			prevInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime.Add(10 * time.Second),
			curInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(11 * time.Second),
			want:     true,
		},
		{
			name: "legacy mode - same tracks, outside time tolerance (too early)",
			prevInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime.Add(10 * time.Second),
			curInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(8*time.Second + 999*time.Millisecond),
			want:     false,
		},
		{
			name: "legacy mode - same tracks, outside time tolerance (too late)",
			prevInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime.Add(10 * time.Second),
			curInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(11*time.Second + 1*time.Millisecond),
			want:     false,
		},
		{
			name: "legacy mode - different number of tracks",
			prevInit: &fmp4.Init{
				Tracks:   baseTracks,
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks:   differentTracks,
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
		{
			name: "legacy mode - different track IDs",
			prevInit: &fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        1,
						TimeScale: 90000,
						Codec: &mcodecs.H264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					},
				},
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        2,
						TimeScale: 90000,
						Codec: &mcodecs.H264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					},
				},
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
		{
			name: "legacy mode - different time scales",
			prevInit: &fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        1,
						TimeScale: 90000,
						Codec: &mcodecs.H264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					},
				},
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        1,
						TimeScale: 48000,
						Codec: &mcodecs.H264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					},
				},
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
		{
			name: "legacy mode - different codec types",
			prevInit: &fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        1,
						TimeScale: 90000,
						Codec: &mcodecs.H264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					},
				},
				UserData: []amp4.IBox{},
			},
			prevEnd: baseTime,
			curInit: &fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        1,
						TimeScale: 90000,
						Codec: &mcodecs.MPEG4Audio{
							Config: mpeg4audio.AudioSpecificConfig{
								Type:         mpeg4audio.ObjectTypeAACLC,
								SampleRate:   48000,
								ChannelCount: 2,
							},
						},
					},
				},
				UserData: []amp4.IBox{},
			},
			curStart: baseTime.Add(5 * time.Second),
			want:     false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := segmentFMP4CanBeConcatenated(tt.prevInit, tt.prevEnd, tt.curInit, tt.curStart)
			require.Equal(t, tt.want, got)
		})
	}
}
