package recorder

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
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
					Type:          2,
					SampleRate:    44100,
					ChannelConfig: 2,
					ChannelCount:  2, //nolint:staticcheck
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

	writeToStream := func(subStream *stream.SubStream, startDTS int64, startNTP time.Time) {
		for i := range 2 {
			pts := startDTS + int64(i)*100*90000/1000
			ntp := startNTP.Add(time.Duration(i*100) * time.Millisecond)

			subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
				PTS: pts,
				NTP: ntp,
				Payload: unit.PayloadH264{
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5}, // IDR
				},
			})

			subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
				PTS: pts,
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
					{0x26, 0x1, 0xaf, 0x8, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2},
				},
			})

			subStream.WriteUnit(desc.Medias[2], desc.Medias[2].Formats[0], &unit.Unit{
				PTS:     pts * int64(desc.Medias[2].Formats[0].ClockRate()) / 90000,
				Payload: unit.PayloadMPEG4Audio{{1, 2, 3, 4}},
			})

			subStream.WriteUnit(desc.Medias[3], desc.Medias[3].Formats[0], &unit.Unit{
				PTS:     pts * int64(desc.Medias[3].Formats[0].ClockRate()) / 90000,
				Payload: unit.PayloadG711{1, 2, 3, 4},
			})

			subStream.WriteUnit(desc.Medias[4], desc.Medias[4].Formats[0], &unit.Unit{
				PTS:     pts * int64(desc.Medias[4].Formats[0].ClockRate()) / 90000,
				Payload: unit.PayloadLPCM{1, 2, 3, 4},
			})
		}
	}

	for _, ca := range []string{"fmp4", "mpegts"} {
		t.Run(ca, func(t *testing.T) {
			strm := &stream.Stream{
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			dir := t.TempDir()

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
						require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-15-27-000000."+ext), segPath)
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
						require.Equal(t, filepath.Join(dir, "mypath", "2008-05-20_22-15-27-000000."+ext), segPath)
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

			writeToStream(subStream,
				50*90000,
				time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC))

			writeToStream(subStream,
				52*90000,
				time.Date(2008, 5, 20, 22, 15, 27, 0, time.UTC))

			// simulate a write error
			subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
				PTS: 0,
				Payload: unit.PayloadH264{
					{5}, // IDR
				},
			})

			for range 2 {
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
							Codec: &mcodecs.H264{
								SPS: test.FormatH264.SPS,
								PPS: test.FormatH264.PPS,
							},
						},
						{
							ID:        2,
							TimeScale: 90000,
							Codec: &mcodecs.H265{
								VPS: []byte{
									0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x60,
									0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03,
									0x00, 0x00, 0x03, 0x00, 0x78, 0xba, 0x02, 0x40,
								},
								SPS: []byte{
									0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
									0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
									0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x11, 0x07,
									0xcb, 0x96, 0xe9, 0x29, 0x30, 0xbc, 0x05, 0xa0,
									0x20, 0x00, 0x00, 0x03, 0x00, 0x20, 0x00, 0x00,
									0x03, 0x03, 0xc1,
								},
								PPS: []byte{
									0x44, 0x01, 0xc0, 0x73, 0xc1, 0x89,
								},
							},
						},
						{
							ID:        3,
							TimeScale: 44100,
							Codec: &mcodecs.MPEG4Audio{
								Config: mpeg4audio.AudioSpecificConfig{
									Type:          2,
									SampleRate:    44100,
									ChannelCount:  2, //nolint:staticcheck
									ChannelConfig: 2,
								},
							},
						},
						{
							ID:        4,
							TimeScale: 8000,
							Codec: &mcodecs.LPCM{
								BitDepth:     16,
								SampleRate:   8000,
								ChannelCount: 1,
							},
						},
						{
							ID:        5,
							TimeScale: 44100,
							Codec: &mcodecs.LPCM{
								BitDepth:     16,
								SampleRate:   44100,
								ChannelCount: 2,
							},
						},
					},
					UserData: []amp4.IBox{
						&recordstore.Mtxi{
							StreamID: init.UserData[0].(*recordstore.Mtxi).StreamID,
							DTS:      50000000000,
							NTP:      1211321725000000000,
						},
					},
				}, init)

				_, err = os.Stat(filepath.Join(dir, "mypath", "2008-05-20_22-15-27-000000."+ext))
				require.NoError(t, err)
			} else {
				_, err = os.Stat(filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000."+ext))
				require.NoError(t, err)

				_, err = os.Stat(filepath.Join(dir, "mypath", "2008-05-20_22-15-27-000000."+ext))
				require.NoError(t, err)
			}

			time.Sleep(50 * time.Millisecond)

			writeToStream(subStream,
				300*90000,
				time.Date(2010, 5, 20, 22, 15, 25, 0, time.UTC))

			time.Sleep(50 * time.Millisecond)

			w.Close()

			<-segCreated
			<-segDone

			_, err = os.Stat(filepath.Join(dir, "mypath", "2010-05-20_22-15-25-000000."+ext))
			require.NoError(t, err)

			if ca == "fmp4" {
				var byts []byte
				byts, err = os.ReadFile(filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000.mp4"))
				require.NoError(t, err)

				var parts fmp4.Parts
				err = parts.Unmarshal(byts)
				require.NoError(t, err)

				for _, part := range parts {
					for _, track := range part.Tracks {
						track.Samples = nil
					}
				}

				require.Equal(t, fmp4.Parts{
					{
						Tracks: []*fmp4.PartTrack{{
							ID: 1,
						}},
					},
					{
						SequenceNumber: 1,
						Tracks: []*fmp4.PartTrack{{
							ID: 2,
						}},
					},
					{
						SequenceNumber: 2,
						Tracks: []*fmp4.PartTrack{{
							ID: 3,
						}},
					},
					{
						SequenceNumber: 3,
						Tracks: []*fmp4.PartTrack{{
							ID: 4,
						}},
					},
					{
						SequenceNumber: 4,
						Tracks: []*fmp4.PartTrack{{
							ID: 5,
						}},
					},
					{
						SequenceNumber: 5,
						Tracks: []*fmp4.PartTrack{{
							ID:       1,
							BaseTime: 9000,
						}},
					},
				}, parts)
			}
		})
	}
}

func TestRecorderFMP4NegativeInitialDTS(t *testing.T) {
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
					Type:          2,
					SampleRate:    44100,
					ChannelConfig: 2,
					ChannelCount:  2, //nolint:staticcheck
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}},
		},
	}}

	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = subStream.Initialize()
	require.NoError(t, err)

	dir := t.TempDir()

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

	for i := range 3 {
		subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS: -50*90000/1000 + (int64(i) * 200 * 90000 / 1000),
			NTP: time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC),
			Payload: unit.PayloadH264{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5}, // IDR
			},
		})

		subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
			PTS:     -100*44100/1000 + (int64(i) * 200 * 44100 / 1000),
			Payload: unit.PayloadMPEG4Audio{{1, 2, 3, 4}},
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
				require.Equal(t, uint64(6615), track.BaseTime)
				found = true
			}
		}
	}

	require.True(t, found)
}

func TestRecorderFMP4NegativeDTSDiff(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type: description.MediaTypeVideo,
			Formats: []rtspformat.Format{&rtspformat.MPEG4Audio{
				PayloadTyp: 96,
				Config: &mpeg4audio.AudioSpecificConfig{
					Type:          2,
					SampleRate:    44100,
					ChannelConfig: 2,
					ChannelCount:  2, //nolint:staticcheck
				},
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}},
		},
	}}

	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = subStream.Initialize()
	require.NoError(t, err)

	dir := t.TempDir()

	recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

	w := &Recorder{
		PathFormat:      recordPath,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     50 * 1024 * 1024,
		SegmentDuration: 2 * time.Second,
		PathName:        "mypath",
		Stream:          strm,
		Parent:          test.NilLogger,
	}
	w.Initialize()

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS:     44100,
		NTP:     time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC),
		Payload: unit.PayloadMPEG4Audio{{1, 2}},
	})

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS:     3 * 44100,
		NTP:     time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC),
		Payload: unit.PayloadMPEG4Audio{{1, 2}},
	})

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS:     2 * 44100,
		NTP:     time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC),
		Payload: unit.PayloadMPEG4Audio{{1, 2}},
	})

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS:     4 * 44100,
		NTP:     time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC),
		Payload: unit.PayloadMPEG4Audio{{1, 2}},
	})

	time.Sleep(50 * time.Millisecond)

	w.Close()

	byts, err := os.ReadFile(filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000.mp4"))
	require.NoError(t, err)

	var parts fmp4.Parts
	err = parts.Unmarshal(byts)
	require.NoError(t, err)

	require.Equal(t, fmp4.Parts{{
		Tracks: []*fmp4.PartTrack{{
			ID: 1,
			Samples: []*fmp4.Sample{
				{
					Payload: []byte{1, 2},
				},
				{
					Duration: 44100,
					Payload:  []byte{1, 2},
				},
			},
		}},
	}}, parts)
}

func TestRecorderSkipTracksPartial(t *testing.T) {
	for _, ca := range []string{"fmp4", "mpegts"} {
		t.Run(ca, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{
				{
					Type:    description.MediaTypeVideo,
					Formats: []rtspformat.Format{&rtspformat.H264{PacketizationMode: 1}},
				},
				{
					Type:    description.MediaTypeVideo,
					Formats: []rtspformat.Format{&rtspformat.VP8{}},
				},
			}}

			strm := &stream.Stream{
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			dir := t.TempDir()

			recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

			n := 0

			l := test.Logger(func(l logger.Level, format string, args ...any) {
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
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			dir := t.TempDir()

			recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

			n := 0

			l := test.Logger(func(l logger.Level, format string, args ...any) {
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
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = subStream.Initialize()
	require.NoError(t, err)

	dir := t.TempDir()

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

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: int64(pts) * 90000 / int64(time.Second),
		NTP: ntp,
		Payload: unit.PayloadH264{
			{5}, // IDR
		},
	})

	pts += 700 * time.Millisecond
	ntp = ntp.Add(700 * time.Millisecond)

	subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{ // segment switch should happen here
		PTS:     int64(pts) * 44100 / int64(time.Second),
		NTP:     ntp,
		Payload: unit.PayloadMPEG4Audio{{1, 2}},
	})

	pts += 400 * time.Millisecond
	ntp = ntp.Add(400 * time.Millisecond)

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: int64(pts) * 90000 / int64(time.Second),
		NTP: ntp,
		Payload: unit.PayloadH264{
			{5}, // IDR
		},
	})

	pts += 100 * time.Millisecond
	ntp = ntp.Add(100 * time.Millisecond)

	subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
		PTS:     int64(pts) * 44100 / int64(time.Second),
		NTP:     ntp,
		Payload: unit.PayloadMPEG4Audio{{3, 4}},
	})

	pts += 400 * time.Millisecond
	ntp = ntp.Add(400 * time.Millisecond)

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: int64(pts) * 90000 / int64(time.Second),
		NTP: ntp,
		Payload: unit.PayloadH264{
			{5}, // IDR
		},
	})

	time.Sleep(100 * time.Millisecond)

	w.Close()

	require.Equal(t, 2, n)
}

func TestRecorderTimeDriftDetector(t *testing.T) {
	for _, ca := range []string{"fmp4", "mpegts"} {
		t.Run(ca, func(t *testing.T) {
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
							Type:          2,
							SampleRate:    44100,
							ChannelConfig: 2,
							ChannelCount:  2, //nolint:staticcheck
						},
						SizeLength:       13,
						IndexLength:      3,
						IndexDeltaLength: 3,
					}},
				},
			}}

			strm := &stream.Stream{
				OrigDesc:          desc,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				Parent:            test.NilLogger,
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &stream.SubStream{
				Stream:        strm,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			dir := t.TempDir()

			recordPath := filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f")

			var ext string
			if ca == "fmp4" {
				ext = "mp4"
			} else {
				ext = "ts"
			}

			segCreated := make(chan struct{}, 10)
			segDone := make(chan struct{}, 10)

			var f conf.RecordFormat
			if ca == "fmp4" {
				f = conf.RecordFormatFMP4
			} else {
				f = conf.RecordFormatMPEGTS
			}

			w := &Recorder{
				PathFormat:      recordPath,
				Format:          f,
				PartDuration:    100 * time.Millisecond,
				MaxPartSize:     50 * 1024 * 1024,
				SegmentDuration: 1 * time.Second,
				PathName:        "mypath",
				Stream:          strm,
				OnSegmentCreate: func(_ string) {
					select {
					case segCreated <- struct{}{}:
					default:
					}
				},
				OnSegmentComplete: func(_ string, _ time.Duration) {
					select {
					case segDone <- struct{}{}:
					default:
					}
				},
				Parent:       test.NilLogger,
				restartPause: 10 * time.Millisecond,
			}
			w.Initialize()

			// Write initial samples with correct timing
			startDTS := int64(50 * 90000)
			startNTP := time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC)

			for i := range 3 {
				pts := startDTS + int64(i)*100*90000/1000
				ntp := startNTP.Add(time.Duration(i*100) * time.Millisecond)

				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					PTS: pts,
					NTP: ntp,
					Payload: unit.PayloadH264{
						test.FormatH264.SPS,
						test.FormatH264.PPS,
						{5}, // IDR
					},
				})

				subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
					PTS:     pts * int64(desc.Medias[1].Formats[0].ClockRate()) / 90000,
					NTP:     ntp,
					Payload: unit.PayloadMPEG4Audio{{1, 2, 3, 4}},
				})
			}

			// Wait for first segment to be created
			select {
			case <-segCreated:
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for first segment")
			}

			// Write more samples to ensure segment has data
			for i := 3; i < 15; i++ {
				pts := startDTS + int64(i)*100*90000/1000
				ntp := startNTP.Add(time.Duration(i*100) * time.Millisecond)

				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					PTS: pts,
					NTP: ntp,
					Payload: unit.PayloadH264{
						test.FormatH264.SPS,
						test.FormatH264.PPS,
						{5}, // IDR
					},
				})

				subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
					PTS:     pts * int64(desc.Medias[1].Formats[0].ClockRate()) / 90000,
					NTP:     ntp,
					Payload: unit.PayloadMPEG4Audio{{1, 2, 3, 4}},
				})
			}

			// Simulate a time drift by advancing NTP time by more than 5 seconds
			// while keeping DTS progression normal (only 100ms forward)
			driftedPTS := startDTS + 15*100*90000/1000
			driftedNTP := startNTP.Add(15*100*time.Millisecond + 6*time.Second) // 6 second drift

			subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
				PTS: driftedPTS,
				NTP: driftedNTP,
				Payload: unit.PayloadH264{
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5}, // IDR
				},
			})

			// Wait for the recorder to detect the drift, complete the segment, and restart
			select {
			case <-segDone:
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for segment completion after drift")
			}

			// Give the recorder time to restart
			time.Sleep(100 * time.Millisecond)

			// Write samples after restart with corrected timing
			restartDTS := int64(60 * 90000)
			restartNTP := time.Date(2008, 5, 20, 22, 15, 35, 0, time.UTC)

			for i := range 3 {
				pts := restartDTS + int64(i)*100*90000/1000
				ntp := restartNTP.Add(time.Duration(i*100) * time.Millisecond)

				subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					PTS: pts,
					NTP: ntp,
					Payload: unit.PayloadH264{
						test.FormatH264.SPS,
						test.FormatH264.PPS,
						{5}, // IDR
					},
				})

				subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
					PTS:     pts * int64(desc.Medias[1].Formats[0].ClockRate()) / 90000,
					NTP:     ntp,
					Payload: unit.PayloadMPEG4Audio{{1, 2, 3, 4}},
				})
			}

			// Wait for second segment to be created after restart
			select {
			case <-segCreated:
			case <-time.After(2 * time.Second):
				t.Fatal("timeout waiting for segment after restart")
			}

			time.Sleep(50 * time.Millisecond)

			w.Close()

			// Wait for final segment to complete
			select {
			case <-segDone:
			case <-time.After(2 * time.Second):
				// This is not fatal as the final segment may complete during Close()
			}

			// Verify that files were created
			entries, err := os.ReadDir(filepath.Join(dir, "mypath"))
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(entries), 2, "expected at least 2 segments (before and after drift)")

			// Verify files have the expected extension
			for _, entry := range entries {
				require.Equal(t, "."+ext, filepath.Ext(entry.Name()))
			}
		})
	}
}

// These tests drive the segment directly rather than a whole stream.
//
// What changed is where a part ends, and that decision is made from a sample's
// flags and the part's duration and size — nothing else. Feeding it a stream
// would mean synthesising a decodable H.264 bitstream, and the test would then
// be exercising the codec parsers rather than the boundary rule.

// alignFixture is a segment writing to a real file, which is what
// closeCurPart needs.
type alignFixture struct {
	segment *formatFMP4Segment
	video   *formatFMP4Track
	audio   *formatFMP4Track
	path    string
}

func newAlignFixture(t *testing.T, align bool, partDuration time.Duration,
	maxPartSize conf.StringSize, hasVideo bool, withAudio bool,
) *alignFixture {
	t.Helper()

	instance := &recorderInstance{
		partDuration:        partDuration,
		partAlignToKeyframe: align,
		maxPartSize:         maxPartSize,
		parent:              &Recorder{Parent: test.NilLogger},
		onSegmentComplete:   func(string, time.Duration) {},
	}

	format := &formatFMP4{ri: instance, hasVideo: hasVideo}

	var video *formatFMP4Track
	if hasVideo {
		video = &formatFMP4Track{
			f: format, id: 1, clockRate: 90000,
			initTrack: &fmp4.InitTrack{
				ID: 1, TimeScale: 90000,
				Codec: &mcodecs.H264{SPS: test.FormatH264.SPS, PPS: test.FormatH264.PPS},
			},
		}
		format.tracks = append(format.tracks, video)
	}

	var audio *formatFMP4Track
	if withAudio {
		audio = &formatFMP4Track{
			f: format, id: 2, clockRate: 44100,
			initTrack: &fmp4.InitTrack{
				ID: 2, TimeScale: 44100,
				Codec: &mcodecs.MPEG4Audio{Config: mpeg4audio.AudioSpecificConfig{
					Type: 2, SampleRate: 44100, ChannelCount: 2, //nolint:staticcheck
				}},
			},
		}
		format.tracks = append(format.tracks, audio)
	}

	path := filepath.Join(t.TempDir(), "segment.mp4")
	file, err := os.Create(path) //nolint:gosec // a test file in a temp dir.
	require.NoError(t, err)
	t.Cleanup(func() { file.Close() })
	require.NoError(t, writeInit(file, uuid.Nil, 0, 0, time.Time{}, format.tracks))

	segment := &formatFMP4Segment{f: format, startDTS: 0, fi: file}
	segment.initialize()

	return &alignFixture{segment: segment, video: video, audio: audio, path: path}
}

// write feeds one sample. payload size is what the size limit is measured in.
func (f *alignFixture) write(
	t *testing.T, track *formatFMP4Track, dts time.Duration, keyframe bool, payload int,
) {
	t.Helper()

	sample := &formatFMP4Sample{
		Sample: &fmp4.Sample{
			Duration:        uint32(40 * int(track.initTrack.TimeScale) / 1000),
			IsNonSyncSample: !keyframe,
			Payload:         make([]byte, payload),
		},
		dts: int64(dts) * int64(track.initTrack.TimeScale) / int64(time.Second),
		ntp: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC).Add(dts),
	}
	require.NoError(t, f.segment.write(track, sample, dts))
}

// parts is how many parts the segment has produced so far, including the open
// one.
func (f *alignFixture) parts() uint32 { return f.segment.nextPartNumber }

// writtenParts closes the segment and reads back the parts from the file, which
// is where the property that matters is visible: the first sample of a part.
func (f *alignFixture) writtenParts(t *testing.T) fmp4.Parts {
	t.Helper()

	require.NoError(t, f.segment.close())

	byts, err := os.ReadFile(f.path)
	require.NoError(t, err)

	var parts fmp4.Parts
	require.NoError(t, parts.Unmarshal(byts))
	return parts
}

// videoStream feeds frames at 40 ms with an IDR every gop frames.
func (f *alignFixture) videoStream(t *testing.T, frames, gop, payload int) {
	t.Helper()

	for i := range frames {
		f.write(t, f.video, time.Duration(i)*40*time.Millisecond, i%gop == 0, payload)
	}
}

// Without the option a part ends as soon as it has lasted long enough, wherever
// that falls. This is the behaviour every existing deployment relies on, and the
// option must not change it.
func TestPartsAreNotAlignedByDefault(t *testing.T) {
	f := newAlignFixture(t, false, 200*time.Millisecond, 50*1024*1024, true, false)

	// 25 frames of 40 ms is one second, with a keyframe only at the start.
	f.videoStream(t, 25, 25, 100)

	// Five parts of 200 ms, and only the first can start at a keyframe.
	require.Equal(t, uint32(5), f.parts())
}

// With the option a part ends only before a sample decoding can start from, so
// every part is independently decodable. This is the whole purpose.
func TestPartsEndOnlyAtKeyframesWhenAligned(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 50*1024*1024, true, false)

	// Four seconds at a one-second GOP.
	f.videoStream(t, 100, 25, 100)

	// One part per GOP: the timer wants a cut every 200 ms and gets one every
	// second, when the next keyframe arrives.
	require.Equal(t, uint32(4), f.parts())

	// And every part written starts at a sample decoding can begin from.
	parts := f.writtenParts(t)
	require.Equal(t, 4, len(parts))
	for _, part := range parts {
		require.False(t, part.Tracks[0].Samples[0].IsNonSyncSample)
	}
}

// A keyframe arriving before the part has lasted long enough does not end it:
// alignment narrows where a part may end, it does not make every keyframe a
// boundary. Otherwise a stream with a short GOP would produce a part per GOP
// regardless of the configured duration.
func TestAKeyframeDoesNotEndAShortPart(t *testing.T) {
	f := newAlignFixture(t, true, 1*time.Second, 50*1024*1024, true, false)

	// A keyframe every 200 ms against a requested part of one second.
	f.videoStream(t, 100, 5, 100)

	// Four seconds, so four parts — each ending at the first keyframe at or
	// after one second, not at every keyframe.
	require.Equal(t, uint32(4), f.parts())
}

// A stream with no video has no random access points to align to. Refusing to
// cut would mean never cutting: the recording would grow into a single part
// until it hit the size limit.
func TestAudioOnlyStreamsStillCutParts(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 50*1024*1024, false, true)

	for i := range 100 {
		f.write(t, f.audio, time.Duration(i)*40*time.Millisecond, false, 100)
	}

	require.Equal(t, uint32(20), f.parts())
}

// An audio sample must not end a part in a stream that has video. Cutting there
// would start the next part with a video sample that is not a random access
// point, which is exactly what alignment exists to prevent.
func TestAudioSamplesDoNotEndAPartInAVideoStream(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 50*1024*1024, true, true)

	// Audio arriving between video frames, with the video GOP at one second.
	for i := range 100 {
		dts := time.Duration(i) * 40 * time.Millisecond
		f.write(t, f.video, dts, i%25 == 0, 100)
		f.write(t, f.audio, dts, false, 20)
	}

	// Still one part per GOP: the audio samples changed nothing.
	require.Equal(t, uint32(4), f.parts())
}

// A camera that stops emitting keyframes must keep recording. The part is ended
// where the size limit falls, which produces one fragment that does not start
// at a random access point — visible to a reader in the sample flags — and that
// is far better than the recording stopping, which is what returning an error
// here would do.
func TestAStreamWithoutKeyframesKeepsRecording(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 4096, true, false)

	// One keyframe at the start and nothing after it, with payloads big enough
	// to reach the limit.
	f.videoStream(t, 100, 1000, 512)

	require.Greater(t, f.parts(), uint32(1),
		"a stream with no keyframes produced one part; the size limit did not end it")
}

// And the same stream without alignment must still fail on the size limit,
// because that is the guard against unbounded memory and this change does not
// remove it.
func TestTheSizeLimitStillFailsWithoutAlignment(t *testing.T) {
	f := newAlignFixture(t, false, 1*time.Hour, 4096, true, false)

	// A part that can never end on duration, so only the limit can stop it.
	var err error
	for i := range 100 {
		sample := &formatFMP4Sample{
			Sample: &fmp4.Sample{
				Duration:        3600,
				IsNonSyncSample: i != 0,
				Payload:         make([]byte, 512),
			},
			dts: int64(i) * 3600,
			ntp: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		}
		if err = f.segment.write(f.video, sample, time.Duration(i)*40*time.Millisecond); err != nil {
			break
		}
	}

	require.Error(t, err, "the size limit stopped guarding against an unbounded part")
	require.Contains(t, err.Error(), "maximum part size")
}
