package mpegts

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/asticode/go-astits"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestToStream(t *testing.T) {
	for _, ca := range []string{
		"h265",
		"h264",
		"mpeg-4 audio latm",
	} {
		t.Run(ca, func(t *testing.T) {
			var buf bytes.Buffer
			mux := astits.NewMuxer(context.Background(), &buf)

			switch ca {
			case "h265":
				err := mux.AddElementaryStream(astits.PMTElementaryStream{
					ElementaryPID: 122,
					StreamType:    astits.StreamTypeH265Video,
				})
				require.NoError(t, err)

				mux.SetPCRPID(122)

				_, err = mux.WriteTables()
				require.NoError(t, err)

			case "h264":
				err := mux.AddElementaryStream(astits.PMTElementaryStream{
					ElementaryPID: 122,
					StreamType:    astits.StreamTypeH264Video,
				})
				require.NoError(t, err)

				mux.SetPCRPID(122)

				_, err = mux.WriteTables()
				require.NoError(t, err)

			case "mpeg-4 audio latm":
				err := mux.AddElementaryStream(astits.PMTElementaryStream{
					ElementaryPID: 122,
					StreamType:    astits.StreamTypeAACLATMAudio,
				})
				require.NoError(t, err)

				mux.SetPCRPID(122)

				enc1, err := mpeg4audio.AudioMuxElement{
					MuxConfigPresent: true,
					StreamMuxConfig: &mpeg4audio.StreamMuxConfig{
						Programs: []*mpeg4audio.StreamMuxConfigProgram{{
							Layers: []*mpeg4audio.StreamMuxConfigLayer{{
								AudioSpecificConfig: &mpeg4audio.AudioSpecificConfig{
									Type:         2,
									SampleRate:   48000,
									ChannelCount: 2,
								},
								LatmBufferFullness: 255,
							}},
						}},
					},
					Payloads: [][][][]byte{{{{1, 2, 3, 4}}}},
				}.Marshal()
				require.NoError(t, err)

				enc2, err := mpeg4audio.AudioSyncStream{
					AudioMuxElements: [][]byte{enc1},
				}.Marshal()
				require.NoError(t, err)

				_, err = mux.WriteData(&astits.MuxerData{
					PID: 122,
					PES: &astits.PESData{
						Header: &astits.PESHeader{
							OptionalHeader: &astits.PESOptionalHeader{
								MarkerBits:      2,
								PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
								PTS:             &astits.ClockReference{Base: 90000},
							},
							StreamID: 192,
						},
						Data: enc2,
					},
				})
				require.NoError(t, err)
			}

			r := &EnhancedReader{R: &buf}
			err := r.Initialize()
			require.NoError(t, err)

			desc, err := ToStream(r, nil, nil)
			require.NoError(t, err)

			switch ca {
			case "h265":
				require.Equal(t, []*description.Media{{
					Type: description.MediaTypeVideo,
					Formats: []format.Format{&format.H265{
						PayloadTyp: 96,
					}},
				}}, desc)

			case "h264":
				require.Equal(t, []*description.Media{{
					Type: description.MediaTypeVideo,
					Formats: []format.Format{&format.H264{
						PayloadTyp:        96,
						PacketizationMode: 1,
					}},
				}}, desc)

			case "mpeg-4 audio latm":
				require.Equal(t, []*description.Media{{
					Type: description.MediaTypeAudio,
					Formats: []format.Format{&format.MPEG4AudioLATM{
						PayloadTyp:     96,
						ProfileLevelID: 30,
						StreamMuxConfig: &mpeg4audio.StreamMuxConfig{
							Programs: []*mpeg4audio.StreamMuxConfigProgram{{
								Layers: []*mpeg4audio.StreamMuxConfigLayer{{
									AudioSpecificConfig: &mpeg4audio.AudioSpecificConfig{
										Type:         2,
										SampleRate:   48000,
										ChannelCount: 2,
									},
									LatmBufferFullness: 255,
								}},
							}},
						},
					}},
				}}, desc)
			}
		})
	}
}

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

	r := &EnhancedReader{R: &buf}
	err = r.Initialize()
	require.NoError(t, err)

	l := test.Logger(func(logger.Level, string, ...interface{}) {
		t.Error("should not happen")
	})
	_, err = ToStream(r, nil, l)
	require.Equal(t, errNoSupportedCodecs, err)
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

	r := &EnhancedReader{R: &buf}
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
