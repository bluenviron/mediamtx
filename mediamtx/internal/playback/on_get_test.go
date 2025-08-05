package playback

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/pmp4"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func writeSegment1(t *testing.T, fpath string) {
	init := fmp4.Init{
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
				TimeScale: 48000,
				Codec: &mp4.CodecMPEG4Audio{
					Config: mpeg4audio.AudioSpecificConfig{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	var buf1 seekablebuffer.Buffer
	err := init.Marshal(&buf1)
	require.NoError(t, err)

	var buf2 seekablebuffer.Buffer
	parts := fmp4.Parts{
		{
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 30 * 90000,
					Samples: []*fmp4.Sample{
						{
							Duration: 30 * 90000,
							Payload:  []byte{1, 2},
						},
						{
							Duration:  1 * 90000,
							PTSOffset: 90000,
							Payload:   []byte{3, 4},
						},
						{
							Duration:        1 * 90000,
							PTSOffset:       -90000,
							IsNonSyncSample: true,
							Payload:         []byte{5, 6},
						}, // 62 secs
					},
				},
				{
					ID:       2,
					BaseTime: 31 * 48000,
					Samples: []*fmp4.Sample{
						{
							Duration: 29 * 48000,
							Payload:  []byte{1, 2},
						},
						{
							Duration: 1 * 48000,
							Payload:  []byte{3, 4},
						}, // 61 secs
					},
				},
			},
		},
	}
	err = parts.Marshal(&buf2)
	require.NoError(t, err)

	err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
	require.NoError(t, err)
}

func writeSegment2(t *testing.T, fpath string) {
	init := fmp4.Init{
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
				TimeScale: 48000,
				Codec: &mp4.CodecMPEG4Audio{
					Config: mpeg4audio.AudioSpecificConfig{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	var buf1 seekablebuffer.Buffer
	err := init.Marshal(&buf1)
	require.NoError(t, err)

	var buf2 seekablebuffer.Buffer
	parts := fmp4.Parts{
		{
			Tracks: []*fmp4.PartTrack{{
				ID:       1,
				BaseTime: 0,
				Samples: []*fmp4.Sample{
					{
						Duration: 1 * 90000,
						Payload:  []byte{7, 8},
					}, // 1 sec
				},
			}},
		},
		{
			Tracks: []*fmp4.PartTrack{{
				ID:       2,
				BaseTime: 0,
				Samples: []*fmp4.Sample{
					{
						Duration: 1 * 48000,
						Payload:  []byte{5, 6},
					},
					{
						Duration: 1 * 48000,
						Payload:  []byte{7, 8},
					}, // 2 secs
				},
			}},
		},
		{
			SequenceNumber: 5,
			Tracks: []*fmp4.PartTrack{{
				ID:       1,
				BaseTime: 1 * 90000,
				Samples: []*fmp4.Sample{
					{
						Duration: 1 * 90000,
						Payload:  []byte{9, 10},
					},
					{
						Duration: 1 * 90000,
						Payload:  []byte{11, 12},
					},
					{
						Duration: 1 * 90000,
						Payload:  []byte{13, 14},
					}, // 4 secs
				},
			}},
		},
	}
	err = parts.Marshal(&buf2)
	require.NoError(t, err)

	err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
	require.NoError(t, err)
}

func writeSegment3(t *testing.T, fpath string) {
	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Codec: &mp4.CodecH264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				},
			},
		},
	}

	var buf1 seekablebuffer.Buffer
	err := init.Marshal(&buf1)
	require.NoError(t, err)

	var buf2 seekablebuffer.Buffer
	parts := fmp4.Parts{
		{
			Tracks: []*fmp4.PartTrack{{
				ID:       1,
				BaseTime: 0,
				Samples: []*fmp4.Sample{
					{
						Duration: 1 * 90000,
						Payload:  []byte{13, 14},
					},
				},
			}},
		},
	}
	err = parts.Marshal(&buf2)
	require.NoError(t, err)

	err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
	require.NoError(t, err)
}

func TestOnGet(t *testing.T) {
	for _, format := range []string{
		"fmp4",
		"mp4",
	} {
		t.Run(format, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "mediamtx-playback")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
			require.NoError(t, err)

			writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
			writeSegment2(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-500000.mp4"))
			writeSegment2(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-04-500000.mp4"))

			s := &Server{
				Address:     "127.0.0.1:9996",
				ReadTimeout: conf.Duration(10 * time.Second),
				PathConfs: map[string]*conf.Path{
					"mypath": {
						Name:       "mypath",
						RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
					},
				},
				AuthManager: test.NilAuthManager,
				Parent:      test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("http://myuser:mypass@localhost:9996/get")
			require.NoError(t, err)

			v := url.Values{}
			v.Set("path", "mypath")
			v.Set("start", time.Date(2008, 11, 0o7, 11, 23, 1, 500000000, time.Local).Format(time.RFC3339Nano))
			v.Set("duration", "3")
			v.Set("format", format)
			u.RawQuery = v.Encode()

			req, err := http.NewRequest(http.MethodGet, u.String(), nil)
			require.NoError(t, err)

			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)

			buf, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			switch format {
			case "fmp4":
				var parts fmp4.Parts
				err = parts.Unmarshal(buf)
				require.NoError(t, err)

				require.Equal(t, fmp4.Parts{
					{
						SequenceNumber: 0,
						Tracks: []*fmp4.PartTrack{
							{
								ID: 1,
								Samples: []*fmp4.Sample{
									{
										Duration:  0,
										PTSOffset: 90000,
										Payload:   []byte{3, 4},
									},
									{
										Duration:        90000,
										PTSOffset:       -90000,
										IsNonSyncSample: true,
										Payload:         []byte{5, 6},
									},
								},
							},
						},
					},
					{
						SequenceNumber: 1,
						Tracks: []*fmp4.PartTrack{
							{
								ID:       2,
								BaseTime: 48000,
								Samples: []*fmp4.Sample{
									{
										Duration: 48000,
										Payload:  []byte{5, 6},
									},
								},
							},
						},
					},
					{
						SequenceNumber: 2,
						Tracks: []*fmp4.PartTrack{
							{
								ID:       1,
								BaseTime: 90000,
								Samples: []*fmp4.Sample{
									{
										Duration: 90000,
										Payload:  []byte{7, 8},
									},
								},
							},
						},
					},
					{
						SequenceNumber: 3,
						Tracks: []*fmp4.PartTrack{
							{
								ID:       1,
								BaseTime: 2 * 90000,
								Samples: []*fmp4.Sample{
									{
										Duration: 90000,
										Payload:  []byte{9, 10},
									},
								},
							},
							{
								ID:       2,
								BaseTime: 2 * 48000,
								Samples: []*fmp4.Sample{
									{
										Duration: 48000,
										Payload:  []byte{7, 8},
									},
								},
							},
						},
					},
				}, parts)

			case "mp4":
				var p pmp4.Presentation
				err = p.Unmarshal(bytes.NewReader(buf))
				require.NoError(t, err)

				sampleData := make(map[int][][]byte)
				for _, track := range p.Tracks {
					var samples [][]byte
					for _, sample := range track.Samples {
						buf, err := sample.GetPayload()
						require.NoError(t, err)
						samples = append(samples, buf)
						sample.GetPayload = nil
					}
					sampleData[track.ID] = samples
				}

				require.Equal(t, pmp4.Presentation{
					Tracks: []*pmp4.Track{
						{
							ID:         1,
							TimeScale:  90000,
							TimeOffset: -90000,
							Codec: &mp4.CodecH264{
								SPS: test.FormatH264.SPS,
								PPS: test.FormatH264.PPS,
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    90000,
									PayloadSize: 2,
								},
								{
									Duration:        90000,
									PTSOffset:       -90000,
									IsNonSyncSample: true,
									PayloadSize:     2,
								},
								{
									Duration:    90000,
									PayloadSize: 2,
								},
								{
									Duration:    90000,
									PayloadSize: 2,
								},
							},
						},
						{
							ID:         2,
							TimeScale:  48000,
							TimeOffset: 48000,
							Codec: &mp4.CodecMPEG4Audio{
								Config: mpeg4audio.AudioSpecificConfig{
									Type:         mpeg4audio.ObjectTypeAACLC,
									SampleRate:   48000,
									ChannelCount: 2,
								},
							},
							Samples: []*pmp4.Sample{
								{
									Duration:    48000,
									PayloadSize: 2,
								},
								{
									Duration:    48000,
									PayloadSize: 2,
								},
							},
						},
					},
				}, p)

				require.Equal(t, map[int][][]byte{
					1: {
						{3, 4},
						{5, 6},
						{7, 8},
						{9, 10},
					},
					2: {
						{5, 6},
						{7, 8},
					},
				}, sampleData)
			}
		})
	}
}

func TestOnGetDifferentInit(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
	writeSegment3(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-500000.mp4"))

	s := &Server{
		Address:     "127.0.0.1:9996",
		ReadTimeout: conf.Duration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:       "mypath",
				RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			},
		},
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://myuser:mypass@localhost:9996/get")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	v.Set("start", time.Date(2008, 11, 0o7, 11, 23, 1, 500000000, time.Local).Format(time.RFC3339Nano))
	v.Set("duration", "2")
	v.Set("format", "fmp4")
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	buf, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	var parts fmp4.Parts
	err = parts.Unmarshal(buf)
	require.NoError(t, err)

	require.Equal(t, fmp4.Parts{
		{
			SequenceNumber: 0,
			Tracks: []*fmp4.PartTrack{
				{
					ID: 1,
					Samples: []*fmp4.Sample{
						{
							Duration:  0,
							PTSOffset: 90000,
							Payload:   []byte{3, 4},
						},
						{
							Duration:        90000,
							PTSOffset:       -90000,
							IsNonSyncSample: true,
							Payload:         []byte{5, 6},
						},
					},
				},
			},
		},
	}, parts)
}

func TestOnGetNTPCompensation(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	writeSegment1(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-500000.mp4"))
	writeSegment2(t, filepath.Join(dir, "mypath", "2008-11-07_11-23-02-000000.mp4")) // remove 0.5 secs

	s := &Server{
		Address:     "127.0.0.1:9996",
		ReadTimeout: conf.Duration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:       "mypath",
				RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
			},
		},
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://myuser:mypass@localhost:9996/get")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	v.Set("start", time.Date(2008, 11, 0o7, 11, 23, 1, 500000000, time.Local).Format(time.RFC3339Nano))
	v.Set("duration", "3")
	v.Set("format", "fmp4")
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	buf, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	var parts fmp4.Parts
	err = parts.Unmarshal(buf)
	require.NoError(t, err)

	require.Equal(t, fmp4.Parts{
		{
			SequenceNumber: 0,
			Tracks: []*fmp4.PartTrack{
				{
					ID: 1,
					Samples: []*fmp4.Sample{
						{
							Duration:  0,
							PTSOffset: 90000,
							Payload:   []byte{3, 4},
						},
						{
							Duration:        90000 - 45000,
							PTSOffset:       -90000,
							IsNonSyncSample: true,
							Payload:         []byte{5, 6},
						},
					},
				},
				{
					ID:       2,
					BaseTime: 24000,
					Samples: []*fmp4.Sample{
						{
							Duration: 48000,
							Payload:  []byte{5, 6},
						},
					},
				},
			},
		},
		{
			SequenceNumber: 1,
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 90000 - 45000,
					Samples: []*fmp4.Sample{
						{
							Duration: 90000,
							Payload:  []byte{7, 8},
						},
					},
				},
			},
		},
		{
			SequenceNumber: 2,
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 2*90000 - 45000,
					Samples: []*fmp4.Sample{
						{
							Duration: 90000,
							Payload:  []byte{9, 10},
						},
					},
				},
			},
		},
		{
			SequenceNumber: 3,
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 225000,
					Samples: []*fmp4.Sample{
						{
							Duration: 90000,
							Payload:  []byte{11, 12},
						},
					},
				},
				{
					ID:       2,
					BaseTime: 72000,
					Samples: []*fmp4.Sample{
						{
							Duration: 48000,
							Payload:  []byte{7, 8},
						},
					},
				},
			},
		},
	}, parts)
}

func TestOnGetInMiddleOfLastSample(t *testing.T) {
	for _, format := range []string{"fmp4", "mp4"} {
		t.Run(format, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "mediamtx-playback")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
			require.NoError(t, err)

			init := fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        1,
						TimeScale: 90000,
						Codec: &mp4.CodecH264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					},
				},
			}

			func() {
				fpath := filepath.Join(dir, "mypath", "2008-11-07_11-22-00-000000.mp4")

				var buf1 seekablebuffer.Buffer
				err = init.Marshal(&buf1)
				require.NoError(t, err)

				var buf2 seekablebuffer.Buffer
				parts := fmp4.Parts{
					{
						Tracks: []*fmp4.PartTrack{
							{
								ID: 1,
								Samples: []*fmp4.Sample{
									{
										Duration:        1 * 90000,
										IsNonSyncSample: false,
										Payload:         []byte{1, 2},
									},
								},
							},
						},
					},
				}
				err = parts.Marshal(&buf2)
				require.NoError(t, err)

				err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
				require.NoError(t, err)
			}()

			s := &Server{
				Address:     "127.0.0.1:9996",
				ReadTimeout: conf.Duration(10 * time.Second),
				PathConfs: map[string]*conf.Path{
					"mypath": {
						Name:       "mypath",
						RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
					},
				},
				AuthManager: test.NilAuthManager,
				Parent:      test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("http://myuser:mypass@localhost:9996/get")
			require.NoError(t, err)

			v := url.Values{}
			v.Set("path", "mypath")
			v.Set("start", time.Date(2008, 11, 7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano))
			v.Set("duration", "3")
			v.Set("format", format)
			u.RawQuery = v.Encode()

			req, err := http.NewRequest(http.MethodGet, u.String(), nil)
			require.NoError(t, err)

			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusNotFound, res.StatusCode)
		})
	}
}

func TestOnGetBetweenSegments(t *testing.T) {
	for _, ca := range []string{
		"idr before",
		"idr after",
	} {
		t.Run(ca, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "mediamtx-playback")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
			require.NoError(t, err)

			init := fmp4.Init{
				Tracks: []*fmp4.InitTrack{
					{
						ID:        1,
						TimeScale: 90000,
						Codec: &mp4.CodecH264{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						},
					},
				},
			}

			func() {
				fpath := filepath.Join(dir, "mypath", "2008-11-07_11-22-00-000000.mp4")

				var buf1 seekablebuffer.Buffer
				err = init.Marshal(&buf1)
				require.NoError(t, err)

				var buf2 seekablebuffer.Buffer
				parts := fmp4.Parts{
					{
						Tracks: []*fmp4.PartTrack{
							{
								ID: 1,
								Samples: []*fmp4.Sample{
									{
										Duration:        1 * 90000,
										IsNonSyncSample: false,
										Payload:         []byte{1, 2},
									},
								},
							},
						},
					},
				}
				err = parts.Marshal(&buf2)
				require.NoError(t, err)

				err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
				require.NoError(t, err)
			}()

			func() {
				fpath := filepath.Join(dir, "mypath", "2008-11-07_11-22-01-000000.mp4")

				var buf1 seekablebuffer.Buffer
				err = init.Marshal(&buf1)
				require.NoError(t, err)

				var buf2 seekablebuffer.Buffer
				parts := fmp4.Parts{
					{
						Tracks: []*fmp4.PartTrack{
							{
								ID: 1,
								Samples: []*fmp4.Sample{
									{
										Duration:        1 * 90000,
										IsNonSyncSample: (ca == "idr before"),
										Payload:         []byte{3, 4},
									},
								},
							},
						},
					},
				}
				err = parts.Marshal(&buf2)
				require.NoError(t, err)

				err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
				require.NoError(t, err)
			}()

			s := &Server{
				Address:     "127.0.0.1:9996",
				ReadTimeout: conf.Duration(10 * time.Second),
				PathConfs: map[string]*conf.Path{
					"mypath": {
						Name:       "mypath",
						RecordPath: filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
					},
				},
				AuthManager: test.NilAuthManager,
				Parent:      test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("http://myuser:mypass@localhost:9996/get")
			require.NoError(t, err)

			v := url.Values{}
			v.Set("path", "mypath")
			v.Set("start", time.Date(2008, 11, 7, 11, 22, 0, 500000000, time.Local).Format(time.RFC3339Nano))
			v.Set("duration", "3")
			v.Set("format", "fmp4")
			u.RawQuery = v.Encode()

			req, err := http.NewRequest(http.MethodGet, u.String(), nil)
			require.NoError(t, err)

			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)

			buf, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			var parts fmp4.Parts
			err = parts.Unmarshal(buf)
			require.NoError(t, err)

			switch ca {
			case "idr before":
				require.Equal(t, fmp4.Parts{
					{
						SequenceNumber: 0,
						Tracks: []*fmp4.PartTrack{
							{
								ID:       1,
								BaseTime: 45000,
								Samples: []*fmp4.Sample{
									{
										Duration: 0,
										Payload:  []byte{1, 2},
									},
									{
										Duration:        90000,
										Payload:         []byte{3, 4},
										IsNonSyncSample: true,
									},
								},
							},
						},
					},
				}, parts)

			case "idr after":
				require.Equal(t, fmp4.Parts{
					{
						SequenceNumber: 0,
						Tracks: []*fmp4.PartTrack{
							{
								ID:       1,
								BaseTime: 45000,
								Samples: []*fmp4.Sample{
									{
										Duration: 90000,
										Payload:  []byte{3, 4},
									},
								},
							},
						},
					},
				}, parts)
			}
		})
	}
}
