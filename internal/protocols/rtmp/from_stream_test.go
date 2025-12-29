package rtmp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/codecprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestFromStream(t *testing.T) {
	h265VPS := []byte{
		0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x60,
		0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x03, 0x00, 0x78, 0xba, 0x02, 0x40,
	}
	h265SPS := []byte{
		0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
		0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
		0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x11, 0x07,
		0xcb, 0x96, 0xe9, 0x29, 0x30, 0xbc, 0x05, 0xa0,
		0x20, 0x00, 0x00, 0x03, 0x00, 0x20, 0x00, 0x00,
		0x03, 0x03, 0xc1,
	}
	h265PPS := []byte{
		0x44, 0x01, 0xc0, 0x73, 0xc1, 0x89,
	}

	cases := []struct {
		name           string
		medias         []*description.Media
		expectedTracks []*gortmplib.Track
		writeUnits     func([]*description.Media, *stream.Stream)
	}{
		{
			name: "h264 + aac",
			medias: []*description.Media{
				{
					Formats: []format.Format{test.FormatH264},
				},
				{
					Formats: []format.Format{test.FormatMPEG4Audio},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.H264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				}},
				{Codec: &codecs.MPEG4Audio{
					Config: test.FormatMPEG4Audio.Config,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
					PTS: 0,
					Payload: unit.PayloadH264{
						{5, 2}, // IDR
					},
				})

				strm.WriteUnit(medias[1], medias[1].Formats[0], &unit.Unit{
					PTS: 90000 * 5,
					Payload: unit.PayloadMPEG4Audio{
						{3, 4},
					},
				})
			},
		},
		{
			name: "av1",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.AV1{
						PayloadTyp: 96,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.AV1{}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 2 * int64(i),
						Payload: unit.PayloadAV1{{
							0x0a, 0x0e, 0x00, 0x00, 0x00, 0x4a, 0xab, 0xbf,
							0xc3, 0x77, 0x6b, 0xe4, 0x40, 0x40, 0x40, 0x41,
						}},
					})
				}
			},
		},
		{
			name: "vp9",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.VP9{
						PayloadTyp: 96,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.VP9{}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS:     90000 * 2 * int64(i),
						Payload: unit.PayloadVP9{1, 2},
					})
				}
			},
		},
		{
			name: "h265",
			medias: []*description.Media{
				{
					Formats: []format.Format{
						&format.H265{
							PayloadTyp: 96,
							VPS:        h265VPS,
							SPS:        h265SPS,
							PPS:        h265PPS,
						},
					},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.H265{
					VPS: h265VPS,
					SPS: h265SPS,
					PPS: h265PPS,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 2 * int64(i),
						Payload: unit.PayloadH265{{
							0x2a, 0x01, 0xad, 0xe0, 0xf5, 0x34, 0x11, 0x0b,
							0x41, 0xe8,
						}},
					})
				}
			},
		},
		{
			name: "h264",
			medias: []*description.Media{
				{
					Formats: []format.Format{test.FormatH264},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.H264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 2 * int64(i),
						Payload: unit.PayloadH264{
							{5, 2}, // IDR
						},
					})
				}
			},
		},
		{
			name: "opus",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.Opus{
						PayloadTyp:   96,
						ChannelCount: 2,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.Opus{
					ChannelCount: 2,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 5 * int64(i),
						Payload: unit.PayloadOpus{
							{3, 4},
						},
					})
				}
			},
		},
		{
			name: "aac",
			medias: []*description.Media{
				{
					Formats: []format.Format{test.FormatMPEG4Audio},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.MPEG4Audio{
					Config: test.FormatMPEG4Audio.Config,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 5 * int64(i),
						Payload: unit.PayloadMPEG4Audio{
							{3, 4},
						},
					})
				}
			},
		},
		{
			name: "mp3",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.MPEG1Audio{}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.MPEG1Audio{}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 5 * int64(i),
						Payload: unit.PayloadMPEG1Audio{
							{
								0xff, 0xfa, 0x52, 0x04, 0x00,
							},
						},
					})
				}
			},
		},
		{
			name: "ac-3",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.AC3{
						SampleRate:   44100,
						ChannelCount: 2,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.AC3{
					SampleRate:   48000,
					ChannelCount: 1,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 5 * int64(i),
						Payload: unit.PayloadAC3{
							{
								0x0b, 0x77, 0x47, 0x11, 0x0c, 0x40, 0x2f, 0x84,
								0x2b, 0xc1, 0x07, 0x7a, 0xb0, 0xfa, 0xbb, 0xea,
								0xef, 0x9f, 0x57, 0x7c, 0xf9, 0xf3, 0xf7, 0xcf,
								0x9f, 0x3e, 0x32, 0xfe, 0xd5, 0xc1, 0x50, 0xde,
								0xc5, 0x1e, 0x73, 0xd2, 0x6c, 0xa6, 0x94, 0x46,
								0x4e, 0x92, 0x8c, 0x0f, 0xb9, 0xcf, 0xad, 0x07,
								0x54, 0x4a, 0x2e, 0xf3, 0x7d, 0x07, 0x2e, 0xa4,
								0x2f, 0xba, 0xbf, 0x39, 0xb5, 0xc9, 0x92, 0xa6,
								0xe1, 0xb4, 0x70, 0xc5, 0xc4, 0xb5, 0xe6, 0x5d,
								0x0f, 0xa8, 0x71, 0xa4, 0xcc, 0xc5, 0xbc, 0x75,
								0x67, 0x92, 0x52, 0x4f, 0x7e, 0x62, 0x1c, 0xa9,
								0xd9, 0xb5, 0x19, 0x6a, 0xd7, 0xb0, 0x44, 0x92,
								0x30, 0x3b, 0xf7, 0x61, 0xd6, 0x49, 0x96, 0x66,
								0x98, 0x28, 0x1a, 0x95, 0xa9, 0x42, 0xad, 0xb7,
								0x50, 0x90, 0xad, 0x1c, 0x34, 0x80, 0xe2, 0xef,
								0xcd, 0x41, 0x0b, 0xf0, 0x9d, 0x57, 0x62, 0x78,
								0xfd, 0xc6, 0xc2, 0x19, 0x9e, 0x26, 0x31, 0xca,
								0x1e, 0x75, 0xb1, 0x7a, 0x8e, 0xb5, 0x51, 0x3a,
								0xfe, 0xe4, 0xf1, 0x0b, 0x4f, 0x14, 0x90, 0xdb,
								0x9f, 0x44, 0x50, 0xbb, 0xef, 0x74, 0x00, 0x8c,
								0x1f, 0x97, 0xa1, 0xa2, 0xfa, 0x72, 0x16, 0x47,
								0xc6, 0xc0, 0xe5, 0xfe, 0x67, 0x03, 0x9c, 0xfe,
								0x62, 0x01, 0xa1, 0x00, 0x5d, 0xff, 0xa5, 0x03,
								0x59, 0xfa, 0xa8, 0x25, 0x5f, 0x6b, 0x83, 0x51,
								0xf2, 0xc0, 0x44, 0xff, 0x2d, 0x05, 0x4b, 0xee,
								0xe0, 0x54, 0x9e, 0xae, 0x86, 0x45, 0xf3, 0xbd,
								0x0e, 0x42, 0xf2, 0xbf, 0x0f, 0x7f, 0xc6, 0x09,
								0x07, 0xdc, 0x22, 0x11, 0x77, 0xbe, 0x31, 0x27,
								0x5b, 0xa4, 0x13, 0x47, 0x07, 0x32, 0x9f, 0x1f,
								0xcb, 0xb0, 0xdf, 0x3e, 0x7d, 0x0d, 0xf3, 0xe7,
								0xcf, 0x9f, 0x3e, 0xae, 0xf9, 0xf3, 0xe7, 0xcf,
								0x9f, 0x3e, 0x85, 0x5d, 0xf3, 0xe7, 0xcf, 0x9f,
								0x3e, 0x7c, 0xf9, 0xf3, 0xe7, 0xcf, 0x9f, 0x3f,
								0x53, 0x5d, 0xf3, 0xe7, 0xcf, 0x9f, 0x3e, 0x7c,
								0xf9, 0xf3, 0xe7, 0xcf, 0x9f, 0x3e, 0x7c, 0xf9,
								0xf3, 0xe7, 0xcf, 0x9f, 0x3e, 0x7c, 0xf9, 0xf3,
								0xe7, 0xcf, 0x9f, 0x3e, 0x00, 0x46, 0x28, 0x26,
								0x20, 0x4a, 0x5a, 0xc0, 0x8a, 0xc5, 0xae, 0xa0,
								0x55, 0x78, 0x82, 0x7a, 0x38, 0x10, 0x09, 0xc9,
								0xb8, 0x0c, 0xfa, 0x5b, 0xc9, 0xd2, 0xec, 0x44,
								0x25, 0xf8, 0x20, 0xf2, 0xc8, 0x8a, 0xe9, 0x40,
								0x18, 0x06, 0xc6, 0x2b, 0xc8, 0xed, 0x8f, 0x33,
								0x09, 0x92, 0x28, 0x1e, 0xc4, 0x24, 0xd8, 0x33,
								0xa5, 0x00, 0xf5, 0xea, 0x18, 0xfa, 0x90, 0x97,
								0x97, 0xe8, 0x39, 0x6a, 0xcf, 0xf1, 0xdd, 0xff,
								0x9e, 0x8e, 0x04, 0x02, 0xae, 0x65, 0x87, 0x5c,
								0x4e, 0x72, 0xfd, 0x3c, 0x01, 0x86, 0xfe, 0x56,
								0x59, 0x74, 0x44, 0x3a, 0x40, 0x00, 0xec, 0xfc,
							},
						},
					})
				}
			},
		},
		{
			name: "pcma",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.G711{
						MULaw:        false,
						SampleRate:   8000,
						ChannelCount: 1,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.G711{
					MULaw:        false,
					ChannelCount: 1,
					SampleRate:   8000,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 5 * int64(i),
						Payload: unit.PayloadG711{
							3, 4,
						},
					})
				}
			},
		},
		{
			name: "pcmu",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.G711{
						MULaw:        true,
						SampleRate:   8000,
						ChannelCount: 1,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.G711{
					MULaw:        true,
					ChannelCount: 1,
					SampleRate:   8000,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 5 * int64(i),
						Payload: unit.PayloadG711{
							3, 4,
						},
					})
				}
			},
		},
		{
			name: "lpcm",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.LPCM{
						BitDepth:     16,
						SampleRate:   44100,
						ChannelCount: 2,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.LPCM{
					BitDepth:     16,
					SampleRate:   44100,
					ChannelCount: 2,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				for i := range 2 {
					strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
						PTS: 90000 * 5 * int64(i),
						Payload: unit.PayloadLPCM{
							3, 4, 5, 6,
						},
					})
				}
			},
		},
		{
			name: "h265 + h264 + vp9 + av1 + opus + aac",
			medias: []*description.Media{
				{
					Formats: []format.Format{&format.H265{}},
				},
				{
					Formats: []format.Format{&format.H264{}},
				},
				{
					Formats: []format.Format{&format.VP9{}},
				},
				{
					Formats: []format.Format{&format.AV1{}},
				},
				{
					Formats: []format.Format{&format.Opus{
						PayloadTyp:   96,
						ChannelCount: 2,
					}},
				},
				{
					Formats: []format.Format{&format.MPEG4Audio{
						PayloadTyp:       96,
						Config:           test.FormatMPEG4Audio.Config,
						SizeLength:       13,
						IndexLength:      3,
						IndexDeltaLength: 3,
					}},
				},
			},
			expectedTracks: []*gortmplib.Track{
				{Codec: &codecs.H265{
					VPS: codecprocessor.H265DefaultVPS,
					SPS: codecprocessor.H265DefaultSPS,
					PPS: codecprocessor.H265DefaultPPS,
				}},
				{Codec: &codecs.H264{
					SPS: codecprocessor.H264DefaultSPS,
					PPS: codecprocessor.H264DefaultPPS,
				}},
				{Codec: &codecs.VP9{}},
				{Codec: &codecs.AV1{}},
				{Codec: &codecs.Opus{
					ChannelCount: 2,
				}},
				{Codec: &codecs.MPEG4Audio{
					Config: test.FormatMPEG4Audio.Config,
				}},
			},
			writeUnits: func(medias []*description.Media, strm *stream.Stream) {
				strm.WriteUnit(medias[0], medias[0].Formats[0], &unit.Unit{
					Payload: unit.PayloadH265{
						{
							0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x60,
							0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03,
							0x00, 0x00, 0x03, 0x00, 0x78, 0xba, 0x02, 0x40,
						},
						{
							0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
							0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
							0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x11, 0x07,
							0xcb, 0x96, 0xe9, 0x29, 0x30, 0xbc, 0x05, 0xa0,
							0x20, 0x00, 0x00, 0x03, 0x00, 0x20, 0x00, 0x00,
							0x03, 0x03, 0xc1,
						},
						{
							0x44, 0x01, 0xc0, 0x73, 0xc1, 0x89,
						},
						{
							0x2a, 0x01, 0xad, 0xe0, 0xf5, 0x34, 0x11, 0x0b,
							0x41, 0xe8,
						},
					},
				})

				strm.WriteUnit(medias[1], medias[1].Formats[0], &unit.Unit{
					Payload: unit.PayloadH264{
						codecprocessor.H264DefaultSPS,
						codecprocessor.H264DefaultPPS,
						{5, 2}, // IDR
					},
				})

				strm.WriteUnit(medias[2], medias[2].Formats[0], &unit.Unit{
					Payload: unit.PayloadVP9{1, 2},
				})

				strm.WriteUnit(medias[3], medias[3].Formats[0], &unit.Unit{
					Payload: unit.PayloadAV1{{
						0x0a, 0x0e, 0x00, 0x00, 0x00, 0x4a, 0xab, 0xbf,
						0xc3, 0x77, 0x6b, 0xe4, 0x40, 0x40, 0x40, 0x41,
					}},
				})

				strm.WriteUnit(medias[4], medias[4].Formats[0], &unit.Unit{
					Payload: unit.PayloadOpus{
						{3, 4},
					},
				})

				strm.WriteUnit(medias[5], medias[5].Formats[0], &unit.Unit{
					PTS: 90000 * 5,
					Payload: unit.PayloadMPEG4Audio{
						{3, 4},
					},
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			medias := tc.medias

			strm := &stream.Stream{
				WriteQueueSize:     512,
				RTPMaxPayloadSize:  1450,
				Desc:               &description.Session{Medias: medias},
				GenerateRTPPackets: true,
				Parent:             test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)

			ln, err := net.Listen("tcp", "127.0.0.1:9121")
			require.NoError(t, err)
			defer ln.Close()

			done := make(chan struct{})

			go func() {
				u, err2 := url.Parse("rtmp://127.0.0.1:9121/stream")
				require.NoError(t, err2)

				c := &gortmplib.Client{
					URL: u,
				}
				err2 = c.Initialize(context.Background())
				require.NoError(t, err2)

				r := &gortmplib.Reader{
					Conn: c,
				}
				err2 = r.Initialize()
				require.NoError(t, err2)

				require.Equal(t, tc.expectedTracks, r.Tracks())

				close(done)
			}()

			nconn, err := ln.Accept()
			require.NoError(t, err)
			defer nconn.Close()

			conn := &gortmplib.ServerConn{
				RW: nconn,
			}
			err = conn.Initialize()
			require.NoError(t, err)

			err = conn.Accept()
			require.NoError(t, err)

			r := &stream.Reader{Parent: test.NilLogger}

			err = FromStream(strm.Desc, r, conn, nconn, 10*time.Second)
			require.NoError(t, err)

			strm.AddReader(r)
			defer strm.RemoveReader(r)

			tc.writeUnits(medias, strm)

			<-done
		})
	}
}

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.VP8{}},
	}}}

	r := &stream.Reader{
		Parent: test.Logger(func(logger.Level, string, ...any) {
			t.Error("should not happen")
		}),
	}

	err := FromStream(desc, r, nil, nil, 0)
	require.Equal(t, errNoSupportedCodecsFrom, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H264{}},
		},
	}}

	n := 0

	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...any) {
			require.Equal(t, logger.Warn, l)
			if n == 0 {
				require.Equal(t, "skipping track 1 (VP8)", fmt.Sprintf(format, args...))
			}
			n++
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:9121")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		u, err2 := url.Parse("rtmp://127.0.0.1:9121/stream")
		require.NoError(t, err2)

		c := &gortmplib.Client{
			URL: u,
		}
		err2 = c.Initialize(context.Background())
		require.NoError(t, err2)
	}()

	nconn, err := ln.Accept()
	require.NoError(t, err)
	defer nconn.Close()

	conn := &gortmplib.ServerConn{
		RW: nconn,
	}
	err = conn.Initialize()
	require.NoError(t, err)

	err = conn.Accept()
	require.NoError(t, err)

	err = FromStream(desc, r, conn, nil, 0)
	require.NoError(t, err)

	require.Equal(t, 1, n)
}
