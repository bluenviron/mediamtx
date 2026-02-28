package stream

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

func TestStreamAlwaysAvailableErrors(t *testing.T) {
	for _, ca := range []struct {
		name   string
		tracks []conf.AlwaysAvailableTrack
		desc   *description.Session
		err    string
	}{
		{
			"wrong tracks",
			[]conf.AlwaysAvailableTrack{
				{Codec: "H264"},
				{Codec: "H265"},
			},
			&description.Session{
				Medias: []*description.Media{
					{
						Type:    description.MediaTypeVideo,
						Formats: []format.Format{&format.H264{}},
					},
				},
			},
			"wants to publish [H264], but stream expects [H264 H265]",
		},
		{
			"wrong mpeg-4 audio config",
			[]conf.AlwaysAvailableTrack{
				{Codec: "MPEG4Audio", SampleRate: 44100, ChannelCount: 2},
			},
			&description.Session{
				Medias: []*description.Media{
					{
						Type: description.MediaTypeAudio,
						Formats: []format.Format{&format.MPEG4Audio{
							Config: &mpeg4audio.AudioSpecificConfig{
								Type:          2,
								SampleRate:    48000,
								ChannelConfig: 1,
							},
						}},
					},
				},
			},
			"MPEG-4 audio configuration does not match, is type=2, sampleRate=48000, " +
				"channelCount=1, but stream expects type=2, sampleRate=44100, channelCount=2",
		},
		{
			"wrong g711 config",
			[]conf.AlwaysAvailableTrack{
				{Codec: "G711", MULaw: true, SampleRate: 8000, ChannelCount: 2},
			},
			&description.Session{
				Medias: []*description.Media{
					{
						Type: description.MediaTypeAudio,
						Formats: []format.Format{&format.G711{
							MULaw:        false,
							SampleRate:   8000,
							ChannelCount: 2,
						}},
					},
				},
			},
			"G711 configuration does not match, is MULaw=false, sampleRate=8000, " +
				"channelCount=2, but stream expects MULaw=true, sampleRate=8000, channelCount=2",
		},
		{
			"wrong lpcm config",
			[]conf.AlwaysAvailableTrack{
				{Codec: "LPCM", SampleRate: 44100, ChannelCount: 2},
			},
			&description.Session{
				Medias: []*description.Media{
					{
						Type: description.MediaTypeAudio,
						Formats: []format.Format{&format.LPCM{
							BitDepth:     16,
							SampleRate:   48000,
							ChannelCount: 2,
						}},
					},
				},
			},
			"LPCM configuration does not match, is bitDepth=16, sampleRate=48000, " +
				"channelCount=2, but stream expects bitDepth=16, sampleRate=44100, channelCount=2",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			strm := &Stream{
				AlwaysAvailable:       true,
				AlwaysAvailableTracks: ca.tracks,
				WriteQueueSize:        512,
				RTPMaxPayloadSize:     1450,
				ReplaceNTP:            true,
				Parent:                &nilLogger{},
			}
			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			subStream := &SubStream{
				Stream:        strm,
				CurDesc:       ca.desc,
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.EqualError(t, err, ca.err)
		})
	}
}

func TestStreamAlwaysAvailable(t *testing.T) {
	for _, ca := range []string{"default", "file"} {
		t.Run(ca, func(t *testing.T) {
			strm := &Stream{
				AlwaysAvailable:   true,
				WriteQueueSize:    512,
				RTPMaxPayloadSize: 1450,
				ReplaceNTP:        true,
				Parent:            &nilLogger{},
			}

			if ca == "default" {
				strm.AlwaysAvailableTracks = []conf.AlwaysAvailableTrack{
					{Codec: conf.CodecAV1},
					{Codec: conf.CodecVP9},
					{Codec: conf.CodecH265},
					{Codec: conf.CodecH264},
					{Codec: conf.CodecOpus},
					{Codec: conf.CodecMPEG4Audio, SampleRate: 44100, ChannelCount: 2},
					{Codec: conf.CodecLPCM, SampleRate: 48000, ChannelCount: 2},
				}
			} else {
				tmpf, err := os.CreateTemp(os.TempDir(), "rtsp-")
				require.NoError(t, err)
				defer os.Remove(tmpf.Name())

				pmp4 := &pmp4.Presentation{
					Tracks: []*pmp4.Track{
						{
							ID:        1,
							TimeScale: 90000,
							Codec: &codecs.AV1{
								SequenceHeader: []byte{8, 0, 0, 0, 66, 167, 191, 228, 96, 13, 0, 64},
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    90000,
									PayloadSize: 13,
									GetPayload: func() ([]byte, error) {
										return []byte{0xa, 0xb, 0x0, 0x0, 0x0, 0x42, 0xa7, 0xbf, 0xe4, 0x60, 0xd, 0x0, 0x40}, nil
									},
								},
							},
						},
						{
							ID:        2,
							TimeScale: 90000,
							Codec: &codecs.VP9{
								Width:             1280,
								Height:            720,
								Profile:           1,
								BitDepth:          8,
								ChromaSubsampling: 1,
								ColorRange:        false,
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    90000,
									PayloadSize: 4,
									GetPayload: func() ([]byte, error) {
										return []byte{1, 2, 3, 4}, nil
									},
								},
							},
						},
						{
							ID:        3,
							TimeScale: 90000,
							Codec: &codecs.H265{
								VPS: []byte{
									0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x02, 0x20,
									0x00, 0x00, 0x03, 0x00, 0xb0, 0x00, 0x00, 0x03,
									0x00, 0x00, 0x03, 0x00, 0x7b, 0x18, 0xb0, 0x24,
								},
								SPS: []byte{
									0x42, 0x01, 0x01, 0x02, 0x20, 0x00, 0x00, 0x03,
									0x00, 0xb0, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
									0x00, 0x7b, 0xa0, 0x07, 0x82, 0x00, 0x88, 0x7d,
									0xb6, 0x71, 0x8b, 0x92, 0x44, 0x80, 0x53, 0x88,
									0x88, 0x92, 0xcf, 0x24, 0xa6, 0x92, 0x72, 0xc9,
									0x12, 0x49, 0x22, 0xdc, 0x91, 0xaa, 0x48, 0xfc,
									0xa2, 0x23, 0xff, 0x00, 0x01, 0x00, 0x01, 0x6a,
									0x02, 0x02, 0x02, 0x01,
								},
								PPS: []byte{
									0x44, 0x01, 0xc0, 0x25, 0x2f, 0x05, 0x32, 0x40,
								},
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    90000,
									PayloadSize: 8,
									GetPayload: func() ([]byte, error) {
										return []byte{0x0, 0x0, 0x0, 0x4, 0x1, 0x2, 0x3, 0x4}, nil
									},
								},
							},
						},
						{
							ID:        4,
							TimeScale: 90000,
							Codec: &codecs.H264{
								SPS: []byte{ // 1920x1080 baseline
									0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
									0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
									0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
								},
								PPS: []byte{0x08, 0x06, 0x07, 0x08},
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    90000,
									PayloadSize: 8,
									GetPayload: func() ([]byte, error) {
										return []byte{0x0, 0x0, 0x0, 0x4, 0x1, 0x2, 0x3, 0x4}, nil
									},
								},
							},
						},
						{
							ID:        5,
							TimeScale: 48000,
							Codec: &codecs.Opus{
								ChannelCount: 2,
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    48000,
									PayloadSize: 2,
									GetPayload: func() ([]byte, error) {
										return []byte{0x1, 0x2}, nil
									},
								},
							},
						},
						{
							ID:        6,
							TimeScale: 44100,
							Codec: &codecs.MPEG4Audio{
								Config: mpeg4audio.AudioSpecificConfig{
									Type:          2,
									SampleRate:    44100,
									ChannelConfig: 2,
									ChannelCount:  2,
								},
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    44100,
									PayloadSize: 4,
									GetPayload: func() ([]byte, error) {
										return []byte{0x12, 0x10, 0x0, 0x0}, nil
									},
								},
							},
						},
						{
							ID:        7,
							TimeScale: 48000,
							Codec: &codecs.LPCM{
								BitDepth:     16,
								SampleRate:   48000,
								ChannelCount: 2,
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    48000,
									PayloadSize: 4,
									GetPayload: func() ([]byte, error) {
										return []byte{0x12, 0x10, 0x0, 0x0}, nil
									},
								},
							},
						},
					},
				}

				err = pmp4.Marshal(tmpf)
				require.NoError(t, err)
				tmpf.Close()

				strm.AlwaysAvailableFile = tmpf.Name()
			}

			err := strm.Initialize()
			require.NoError(t, err)
			defer strm.Close()

			r := &Reader{
				Parent: &nilLogger{},
			}

			var wg sync.WaitGroup
			n := 0
			var phase2 atomic.Bool

			wg.Add(1)
			var lastPTSAV1 int64
			var soAV1a sync.Once
			var soAV1b sync.Once
			r.OnData(strm.Desc.Medias[n], strm.Desc.Medias[n].Formats[0], func(u *unit.Unit) error {
				require.GreaterOrEqual(t, u.PTS, lastPTSAV1)
				lastPTSAV1 = u.PTS

				if !phase2.Load() {
					soAV1a.Do(func() {
						require.NotEmpty(t, u.Payload)
						wg.Done()
					})
				} else {
					soAV1b.Do(func() {
						require.Equal(t, unit.PayloadAV1{{1, 2, 3, 4}}, u.Payload)
						wg.Done()
					})
				}
				return nil
			})
			n++

			wg.Add(1)
			var lastPTSVP9 int64
			var soVP9a sync.Once
			var soVP9b sync.Once
			r.OnData(strm.Desc.Medias[n], strm.Desc.Medias[n].Formats[0], func(u *unit.Unit) error {
				require.GreaterOrEqual(t, u.PTS, lastPTSVP9)
				lastPTSVP9 = u.PTS

				if !phase2.Load() {
					soVP9a.Do(func() {
						require.NotEmpty(t, u.Payload)
						wg.Done()
					})
				} else {
					soVP9b.Do(func() {
						require.Equal(t, unit.PayloadVP9{1, 2, 3, 4}, u.Payload)
						wg.Done()
					})
				}
				return nil
			})
			n++

			wg.Add(1)
			var lastPTSH265 int64
			var soH265a sync.Once
			var soH265b sync.Once
			r.OnData(strm.Desc.Medias[n], strm.Desc.Medias[n].Formats[0], func(u *unit.Unit) error {
				require.GreaterOrEqual(t, u.PTS, lastPTSH265)
				lastPTSH265 = u.PTS

				if !phase2.Load() {
					soH265a.Do(func() {
						require.NotEmpty(t, u.Payload)
						wg.Done()
					})
				} else {
					soH265b.Do(func() {
						require.Equal(t, unit.PayloadH265{{1, 2, 3, 4}}, u.Payload)
						wg.Done()
					})
				}
				return nil
			})
			n++

			wg.Add(1)
			var lastPTSH264 int64
			var soH264a sync.Once
			var soH264b sync.Once
			r.OnData(strm.Desc.Medias[n], strm.Desc.Medias[n].Formats[0], func(u *unit.Unit) error {
				require.GreaterOrEqual(t, u.PTS, lastPTSH264)
				lastPTSH264 = u.PTS

				if !phase2.Load() {
					soH264a.Do(func() {
						require.NotEmpty(t, u.Payload)
						wg.Done()
					})
				} else {
					soH264b.Do(func() {
						require.Equal(t, unit.PayloadH264{{1, 2, 3, 4}}, u.Payload)
						wg.Done()
					})
				}
				return nil
			})
			n++

			wg.Add(1)
			var lastPTSOpus int64
			var soOpusa sync.Once
			var soOpusb sync.Once
			r.OnData(strm.Desc.Medias[n], strm.Desc.Medias[n].Formats[0], func(u *unit.Unit) error {
				require.GreaterOrEqual(t, u.PTS, lastPTSOpus)
				lastPTSOpus = u.PTS

				if !phase2.Load() {
					soOpusa.Do(func() {
						require.NotEmpty(t, u.Payload)
						wg.Done()
					})
				} else {
					soOpusb.Do(func() {
						require.Equal(t, unit.PayloadOpus{{1, 2}}, u.Payload)
						wg.Done()
					})
				}
				return nil
			})
			n++

			wg.Add(1)
			var lastPTSMPEG4Audio int64
			var soMPEG4Audioa sync.Once
			var soMPEG4Audiob sync.Once
			r.OnData(strm.Desc.Medias[n], strm.Desc.Medias[n].Formats[0], func(u *unit.Unit) error {
				require.GreaterOrEqual(t, u.PTS, lastPTSMPEG4Audio)
				lastPTSMPEG4Audio = u.PTS

				if !phase2.Load() {
					soMPEG4Audioa.Do(func() {
						require.NotEmpty(t, u.Payload)
						wg.Done()
					})
				} else {
					soMPEG4Audiob.Do(func() {
						require.Equal(t, unit.PayloadMPEG4Audio{{1, 2, 3, 4}}, u.Payload)
						wg.Done()
					})
				}
				return nil
			})
			n++

			wg.Add(1)
			var lastPTSLPCM int64
			var soLPCMa sync.Once
			var soLPCMb sync.Once
			r.OnData(strm.Desc.Medias[n], strm.Desc.Medias[n].Formats[0], func(u *unit.Unit) error {
				require.GreaterOrEqual(t, u.PTS, lastPTSLPCM)
				lastPTSLPCM = u.PTS

				if !phase2.Load() {
					soLPCMa.Do(func() {
						require.NotEmpty(t, u.Payload)
						wg.Done()
					})
				} else {
					soLPCMb.Do(func() {
						require.Equal(t, unit.PayloadLPCM{1, 2, 3, 4}, u.Payload)
						wg.Done()
					})
				}
				return nil
			})

			strm.AddReader(r)
			defer strm.RemoveReader(r)

			wg.Wait()

			subStream := &SubStream{
				Stream: strm,
				CurDesc: &description.Session{Medias: []*description.Media{
					{
						Type:    description.MediaTypeVideo,
						Formats: []format.Format{&format.AV1{}},
					},
					{
						Type:    description.MediaTypeVideo,
						Formats: []format.Format{&format.VP9{}},
					},
					{
						Type:    description.MediaTypeVideo,
						Formats: []format.Format{&format.H265{}},
					},
					{
						Type:    description.MediaTypeVideo,
						Formats: []format.Format{&format.H264{}},
					},
					{
						Type:    description.MediaTypeAudio,
						Formats: []format.Format{&format.Opus{}},
					},
					{
						Type: description.MediaTypeAudio,
						Formats: []format.Format{&format.MPEG4Audio{
							Config: &mpeg4audio.AudioSpecificConfig{
								Type:          2,
								SampleRate:    44100,
								ChannelConfig: 2,
								ChannelCount:  2,
							},
						}},
					},
					{
						Type: description.MediaTypeAudio,
						Formats: []format.Format{&format.LPCM{
							BitDepth:     16,
							SampleRate:   48000,
							ChannelCount: 2,
						}},
					},
				}},
				UseRTPPackets: false,
			}
			err = subStream.Initialize()
			require.NoError(t, err)

			wg.Add(7)
			phase2.Store(true)

			subStream.WriteUnit(subStream.CurDesc.Medias[0], subStream.CurDesc.Medias[0].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadAV1{{1, 2, 3, 4}},
			})

			subStream.WriteUnit(subStream.CurDesc.Medias[1], subStream.CurDesc.Medias[1].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadVP9{1, 2, 3, 4},
			})

			subStream.WriteUnit(subStream.CurDesc.Medias[2], subStream.CurDesc.Medias[2].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadH265{{1, 2, 3, 4}},
			})

			subStream.WriteUnit(subStream.CurDesc.Medias[3], subStream.CurDesc.Medias[3].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadH264{{1, 2, 3, 4}},
			})

			subStream.WriteUnit(subStream.CurDesc.Medias[4], subStream.CurDesc.Medias[4].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadOpus{{1, 2}},
			})

			subStream.WriteUnit(subStream.CurDesc.Medias[5], subStream.CurDesc.Medias[5].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadMPEG4Audio{{1, 2, 3, 4}},
			})

			subStream.WriteUnit(subStream.CurDesc.Medias[6], subStream.CurDesc.Medias[6].Formats[0], &unit.Unit{
				PTS:     0,
				Payload: unit.PayloadLPCM{1, 2, 3, 4},
			})

			wg.Wait()
		})
	}
}
