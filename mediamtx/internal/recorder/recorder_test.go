package recorder

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestRecorder(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type: description.MediaTypeVideo,
			Formats: []rtspformat.Format{&rtspformat.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
			}},
		},
		{
			Type: description.MediaTypeVideo,
			Formats: []rtspformat.Format{&rtspformat.H265{
				PayloadTyp: 96,
			}},
		},
		{
			Type: description.MediaTypeAudio,
			Formats: []rtspformat.Format{&rtspformat.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.AudioSpecificConfig{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}},
		},
		{
			Type: description.MediaTypeAudio,
			Formats: []rtspformat.Format{&rtspformat.G711{
				PayloadTyp:   8,
				MULaw:        false,
				SampleRate:   8000,
				ChannelCount: 1,
			}},
		},
		{
			Type: description.MediaTypeAudio,
			Formats: []rtspformat.Format{&rtspformat.LPCM{
				PayloadTyp:   96,
				BitDepth:     16,
				SampleRate:   44100,
				ChannelCount: 2,
			}},
		},
	}}

	writeToStream := func(strm *stream.Stream, startDTS int64, startNTP time.Time) {
		for i := 0; i < 2; i++ {
			pts := startDTS + int64(i)*100*90000/1000
			ntp := startNTP.Add(time.Duration(i*60) * time.Second)

			strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
				Base: unit.Base{
					PTS: pts,
					NTP: ntp,
				},
				AU: [][]byte{
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5}, // IDR
				},
			})

			strm.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.H265{
				Base: unit.Base{
					PTS: pts,
				},
				AU: [][]byte{
					test.FormatH265.VPS,
					test.FormatH265.SPS,
					test.FormatH265.PPS,
					{byte(h265.NALUType_CRA_NUT) << 1, 0}, // IDR
				},
			})

			strm.WriteUnit(desc.Medias[2], desc.Medias[2].Formats[0], &unit.MPEG4Audio{
				Base: unit.Base{
					PTS: pts * int64(desc.Medias[2].Formats[0].ClockRate()) / 90000,
				},
				AUs: [][]byte{{1, 2, 3, 4}},
			})

			strm.WriteUnit(desc.Medias[3], desc.Medias[3].Formats[0], &unit.G711{
				Base: unit.Base{
					PTS: pts * int64(desc.Medias[3].Formats[0].ClockRate()) / 90000,
				},
				Samples: []byte{1, 2, 3, 4},
			})

			strm.WriteUnit(desc.Medias[4], desc.Medias[4].Formats[0], &unit.LPCM{
				Base: unit.Base{
					PTS: pts * int64(desc.Medias[4].Formats[0].ClockRate()) / 90000,
				},
				Samples: []byte{1, 2, 3, 4},
			})
		}
	}

	for _, ca := range []string{"fmp4", "mpegts"} {
		t.Run(ca, func(t *testing.T) {
			strm := &stream.Stream{
				WriteQueueSize:     512,
				RTPMaxPayloadSize:  1450,
				Desc:               desc,
				GenerateRTPPackets: true,
				Parent:             test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			dir, err := os.MkdirTemp("", "mediamtx-agent")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

			segCreated := make(chan struct{}, 4)
			segDone := make(chan struct{}, 4)

			var f conf.RecordFormat
			if ca == "fmp4" {
				f = conf.RecordFormatFMP4
			} else {
				f = conf.RecordFormatMPEGTS
			}

			var ext string
			if ca == "fmp4" {
				ext = "mp4"
			} else {
				ext = "ts"
			}

			n := 0

			w := &Recorder{
				PathFormat:      recordPath,
				Format:          f,
				PartDuration:    100 * time.Millisecond,
				MaxPartSize:     50 * 1024 * 1024,
				SegmentDuration: 1 * time.Second,
				PathName:        "mypath",
				Stream:          strm,
				OnSegmentCreate: func(segPath string) {
					switch n {
					case 0:
						require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000."+ext), segPath)
					case 1:
						require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-16-25-000000."+ext), segPath)
					default:
						require.Equal(t, filepath.Join(dir, "mypath", "2010-05-20_22-15-25-000000."+ext), segPath)
					}
					segCreated <- struct{}{}
				},
				OnSegmentComplete: func(segPath string, du time.Duration) {
					switch n {
					case 0:
						require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000."+ext), segPath)
						require.Equal(t, 2*time.Second, du)
					case 1:
						require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-16-25-000000."+ext), segPath)
						require.Equal(t, 100*time.Millisecond, du)
					default:
						require.Equal(t, filepath.Join(dir, "mypath", "2010-05-20_22-15-25-000000."+ext), segPath)
						require.Equal(t, 100*time.Millisecond, du)
					}
					n++
					segDone <- struct{}{}
				},
				Parent:       test.NilLogger,
				restartPause: 1 * time.Millisecond,
			}
			w.Initialize()

			writeToStream(strm,
				50*90000,
				time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC))

			writeToStream(strm,
				52*90000,
				time.Date(2008, 5, 20, 22, 16, 25, 0, time.UTC))

			// simulate a write error
			strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
				Base: unit.Base{
					PTS: 0,
				},
				AU: [][]byte{
					{5}, // IDR
				},
			})

			for i := 0; i < 2; i++ {
				<-segCreated
				<-segDone
			}

			if ca == "fmp4" {
				var init fmp4.Init

				func() {
					f, err2 := os.Open(filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000."+ext))
					require.NoError(t, err2)
					defer f.Close()

					err2 = init.Unmarshal(f)
					require.NoError(t, err2)
				}()

				require.Equal(t, fmp4.Init{
					Tracks: []*fmp4.InitTrack{
						{
							ID:        1,
							TimeScale: 90000,
							Codec: &mp4.CodecH264{
								SPS: test.FormatH264.SPS,
								PPS: test.FormatH264.PPS,
							},
						},
						{
							ID:        2,
							TimeScale: 90000,
							Codec: &mp4.CodecH265{
								VPS: test.FormatH265.VPS,
								SPS: test.FormatH265.SPS,
								PPS: test.FormatH265.PPS,
							},
						},
						{
							ID:        3,
							TimeScale: 44100,
							Codec: &mp4.CodecMPEG4Audio{
								Config: mpeg4audio.AudioSpecificConfig{
									Type:         2,
									SampleRate:   44100,
									ChannelCount: 2,
								},
							},
						},
						{
							ID:        4,
							TimeScale: 8000,
							Codec: &mp4.CodecLPCM{
								BitDepth:     16,
								SampleRate:   8000,
								ChannelCount: 1,
							},
						},
						{
							ID:        5,
							TimeScale: 44100,
							Codec: &mp4.CodecLPCM{
								BitDepth:     16,
								SampleRate:   44100,
								ChannelCount: 2,
							},
						},
					},
				}, init)

				_, err = os.Stat(filepath.Join(dir, "mypath", "2008-05-20_22-16-25-000000."+ext))
				require.NoError(t, err)
			} else {
				_, err = os.Stat(filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000."+ext))
				require.NoError(t, err)

				_, err = os.Stat(filepath.Join(dir, "mypath", "2008-05-20_22-16-25-000000."+ext))
				require.NoError(t, err)
			}

			time.Sleep(50 * time.Millisecond)

			writeToStream(strm,
				300*90000,
				time.Date(2010, 5, 20, 22, 15, 25, 0, time.UTC))

			time.Sleep(50 * time.Millisecond)

			w.Close()

			<-segCreated
			<-segDone

			_, err = os.Stat(filepath.Join(dir, "mypath", "2010-05-20_22-15-25-000000."+ext))
			require.NoError(t, err)
		})
	}
}

func TestRecorderFMP4NegativeDTS(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type: description.MediaTypeVideo,
			Formats: []rtspformat.Format{&rtspformat.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
			}},
		},
		{
			Type: description.MediaTypeAudio,
			Formats: []rtspformat.Format{&rtspformat.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.AudioSpecificConfig{
					Type:         2,
					SampleRate:   44100,
					ChannelCount: 2,
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}},
		},
	}}

	strm := &stream.Stream{
		WriteQueueSize:     512,
		RTPMaxPayloadSize:  1450,
		Desc:               desc,
		GenerateRTPPackets: true,
		Parent:             test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	dir, err := os.MkdirTemp("", "mediamtx-agent")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

	w := &Recorder{
		PathFormat:      recordPath,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     50 * 1024 * 1024,
		SegmentDuration: 1 * time.Second,
		PathName:        "mypath",
		Stream:          strm,
		Parent:          test.NilLogger,
	}
	w.Initialize()

	for i := 0; i < 3; i++ {
		strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
			Base: unit.Base{
				PTS: -50*90000/1000 + (int64(i) * 200 * 90000 / 1000),
				NTP: time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC),
			},
			AU: [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5}, // IDR
			},
		})

		strm.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.MPEG4Audio{
			Base: unit.Base{
				PTS: -100*44100/1000 + (int64(i) * 200 * 44100 / 1000),
			},
			AUs: [][]byte{{1, 2, 3, 4}},
		})
	}

	time.Sleep(50 * time.Millisecond)

	w.Close()

	byts, err := os.ReadFile(filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000.mp4"))
	require.NoError(t, err)

	var parts fmp4.Parts
	err = parts.Unmarshal(byts)
	require.NoError(t, err)

	found := false

	for _, part := range parts {
		for _, track := range part.Tracks {
			if track.ID == 2 {
				require.Less(t, track.BaseTime, uint64(1*90000))
				found = true
			}
		}
	}

	require.True(t, found)
}

func TestRecorderSkipTracksPartial(t *testing.T) {
	for _, ca := range []string{"fmp4", "mpegts"} {
		t.Run(ca, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{
				{
					Type:    description.MediaTypeVideo,
					Formats: []rtspformat.Format{&rtspformat.H264{}},
				},
				{
					Type:    description.MediaTypeVideo,
					Formats: []rtspformat.Format{&rtspformat.VP8{}},
				},
			}}

			strm := &stream.Stream{
				WriteQueueSize:     512,
				RTPMaxPayloadSize:  1450,
				Desc:               desc,
				GenerateRTPPackets: true,
				Parent:             test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			dir, err := os.MkdirTemp("", "mediamtx-agent")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

			n := 0

			l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
				if n == 0 {
					require.Equal(t, logger.Warn, l)
					require.Equal(t, "[recorder] skipping track 2 (VP8)", fmt.Sprintf(format, args...))
				}
				n++
			})

			var fo conf.RecordFormat
			if ca == "fmp4" {
				fo = conf.RecordFormatFMP4
			} else {
				fo = conf.RecordFormatMPEGTS
			}

			w := &Recorder{
				PathFormat:      recordPath,
				Format:          fo,
				PartDuration:    100 * time.Millisecond,
				MaxPartSize:     50 * 1024 * 1024,
				SegmentDuration: 1 * time.Second,
				PathName:        "mypath",
				Stream:          strm,
				Parent:          l,
			}
			w.Initialize()
			defer w.Close()

			require.Equal(t, 2, n)
		})
	}
}

func TestRecorderSkipTracksFull(t *testing.T) {
	for _, ca := range []string{"fmp4", "mpegts"} {
		t.Run(ca, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{
				{
					Type:    description.MediaTypeVideo,
					Formats: []rtspformat.Format{&rtspformat.VP8{}},
				},
			}}

			strm := &stream.Stream{
				WriteQueueSize:     512,
				RTPMaxPayloadSize:  1450,
				Desc:               desc,
				GenerateRTPPackets: true,
				Parent:             test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			dir, err := os.MkdirTemp("", "mediamtx-agent")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

			n := 0

			l := test.Logger(func(l logger.Level, format string, args ...interface{}) {
				if n == 0 {
					require.Equal(t, logger.Warn, l)
					require.Equal(t, "[recorder] no supported tracks found, skipping recording", fmt.Sprintf(format, args...))
				}
				n++
			})

			var fo conf.RecordFormat
			if ca == "fmp4" {
				fo = conf.RecordFormatFMP4
			} else {
				fo = conf.RecordFormatMPEGTS
			}

			w := &Recorder{
				PathFormat:      recordPath,
				Format:          fo,
				PartDuration:    100 * time.Millisecond,
				MaxPartSize:     50 * 1024 * 1024,
				SegmentDuration: 1 * time.Second,
				PathName:        "mypath",
				Stream:          strm,
				Parent:          l,
			}
			w.Initialize()
			defer w.Close()

			require.Equal(t, 1, n)
		})
	}
}

func TestRecorderFMP4SegmentSwitch(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []rtspformat.Format{test.FormatH264},
		},
		{
			Type:    description.MediaTypeAudio,
			Formats: []rtspformat.Format{test.FormatMPEG4Audio},
		},
	}}

	strm := &stream.Stream{
		WriteQueueSize:     512,
		RTPMaxPayloadSize:  1450,
		Desc:               desc,
		GenerateRTPPackets: true,
		Parent:             test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	dir, err := os.MkdirTemp("", "mediamtx-agent")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	n := 0

	w := &Recorder{
		PathFormat:      filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     50 * 1024 * 1024,
		SegmentDuration: 1 * time.Second,
		PathName:        "mypath",
		Stream:          strm,
		Parent:          test.NilLogger,
		OnSegmentCreate: func(segPath string) {
			switch n {
			case 0:
				require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000.mp4"), segPath)
			case 1:
				require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-15-25-700000.mp4"), segPath) // +0.7s
			}
			n++
		},
	}
	w.Initialize()

	pts := 50 * time.Second
	ntp := time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
		Base: unit.Base{
			PTS: int64(pts) * 90000 / int64(time.Second),
			NTP: ntp,
		},
		AU: [][]byte{
			{5}, // IDR
		},
	})

	pts += 700 * time.Millisecond
	ntp = ntp.Add(700 * time.Millisecond)

	strm.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.MPEG4Audio{ // segment switch should happen here
		Base: unit.Base{
			PTS: int64(pts) * 44100 / int64(time.Second),
			NTP: ntp,
		},
		AUs: [][]byte{{1, 2}},
	})

	pts += 400 * time.Millisecond
	ntp = ntp.Add(400 * time.Millisecond)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
		Base: unit.Base{
			PTS: int64(pts) * 90000 / int64(time.Second),
			NTP: ntp,
		},
		AU: [][]byte{
			{5}, // IDR
		},
	})

	pts += 100 * time.Millisecond
	ntp = ntp.Add(100 * time.Millisecond)

	strm.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.MPEG4Audio{
		Base: unit.Base{
			PTS: int64(pts) * 44100 / int64(time.Second),
			NTP: ntp,
		},
		AUs: [][]byte{{3, 4}},
	})

	pts += 400 * time.Millisecond
	ntp = ntp.Add(400 * time.Millisecond)

	strm.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
		Base: unit.Base{
			PTS: int64(pts) * 90000 / int64(time.Second),
			NTP: ntp,
		},
		AU: [][]byte{
			{5}, // IDR
		},
	})

	time.Sleep(100 * time.Millisecond)

	w.Close()

	require.Equal(t, 2, n)
}
