package recorder

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/test"
)

func startupFixture(t *testing.T, videoConfigured bool) (*formatFMP4, *formatFMP4Track, *formatFMP4Track) {
	t.Helper()
	f := &formatFMP4{
		ri: &recorderInstance{
			pathFormat2: filepath.Join(t.TempDir(),
				"%Y-%m-%d_%H-%M-%S-%f.mp4"),
			location:        time.UTC,
			partDuration:    100 * time.Millisecond,
			maxPartSize:     1024,
			segmentDuration: 60 * time.Second,
			parent:          test.NilLogger,
			onSegmentCreate: func(string) {
			},
			onSegmentComplete: func(string,
				time.Duration) {
			},
		},
	}
	a := &formatFMP4Track{
		f:         f,
		id:        2,
		clockRate: 8000,
		codec: &mcodecs.LPCM{
			SampleRate:   8000,
			BitDepth:     16,
			ChannelCount: 1,
		},
	}
	v := &formatFMP4Track{
		f:         f,
		id:        1,
		clockRate: 90000,
		codec: &mcodecs.H264{
			SPS: test.FormatH264.SPS,
			PPS: test.FormatH264.PPS,
		},
	}
	a.initialize()
	v.initialize()
	f.tracks = []*formatFMP4Track{a}
	if videoConfigured {
		f.tracks = append(f.tracks, v)
	}
	t.Cleanup(f.close)
	return f, a, v
}

func TestStartupPreRollBoundAndFlush(t *testing.T) {
	f, a, v := startupFixture(t, true)
	ntp := time.Unix(1700000000, 0)
	for i := range 4 {
		require.NoError(t,
			a.write(&formatFMP4Sample{
				Sample: &fmp4.Sample{
					Payload: make([]byte,
						128),
				},
				dts: int64(50*8000 + i*320),
				ntp: ntp.Add(time.Duration(i) * 40 * time.Millisecond),
			}))
	}
	require.Nil(t, f.currentSegment)
	require.Len(t, f.initialSamples, 3)
	require.Equal(t, uint64(768), f.initialSize)
	require.NoError(t,
		v.write(&formatFMP4Sample{
			Sample: &fmp4.Sample{
				Payload: []byte{
					0,
				},
			},
			dts: 51 * 90000,
			ntp: ntp.Add(time.Second),
		}))
	require.NoError(t,
		v.write(&formatFMP4Sample{
			Sample: &fmp4.Sample{
				Payload: []byte{
					1,
				},
				IsNonSyncSample: true,
			},
			dts: 51*90000 + 3600,
			ntp: ntp.Add(1040 * time.Millisecond),
		}))
	require.NotNil(t, f.currentSegment)
	require.Equal(t, 50*time.Second, f.currentSegment.startDTS)
	require.Empty(t, f.initialSamples)
	require.Zero(t, f.initialSize)
}

func TestStartupPreRollOverflowIncludingEmptyPayload(t *testing.T) {
	for _, payloadSize := range []int{0, 128} {
		t.Run(time.Duration(payloadSize).String(), func(t *testing.T) {
			f, a, _ := startupFixture(t, true)
			ntp := time.Unix(1700000000, 0)
			for i := range 20 {
				err := a.write(&formatFMP4Sample{
					Sample: &fmp4.Sample{
						Payload: make([]byte,
							payloadSize),
					},
					dts: int64(i * 320),
					ntp: ntp.Add(time.Duration(i) * 40 * time.Millisecond),
				})
				if err != nil {
					require.ErrorContains(t, err, "maximum startup pre-roll")
					require.Nil(t, f.currentSegment)
					require.Empty(t, f.initialSamples)
					require.Zero(t, f.initialSize)
					return
				}
			}
			t.Fatal("startup buffer did not reject overflow")
		})
	}
}

func TestStartupAudioOnlyDoesNotWaitForVideo(t *testing.T) {
	f, a, _ := startupFixture(t, false)
	ntp := time.Unix(1700000000, 0)
	for i := range 2 {
		require.NoError(t,
			a.write(&formatFMP4Sample{
				Sample: &fmp4.Sample{
					Payload: []byte{
						1,
					},
				},
				dts: int64(i * 320),
				ntp: ntp.Add(time.Duration(i) * 40 * time.Millisecond),
			}))
	}
	require.NotNil(t, f.currentSegment)
	require.Empty(t, f.initialSamples)
}
