package recorder

import (
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"
)

func TestRecorderLPCMResumeReservationBound(t *testing.T) {
	ntp := time.Unix(1700000000, 0)
	video := &formatFMP4Track{
		initTrack: &fmp4.InitTrack{
			TimeScale: 90000,
			Codec:     &mcodecs.H264{},
		},
		nextSample: &formatFMP4Sample{
			dts: 90 * 90000,
			ntp: ntp,
		},
	}
	for _, ca := range []struct {
		name                string
		audioDTS, age, want time.Duration
	}{
		{"lag1.5", 10 * time.Second, 78500 * time.Millisecond, 87500 * time.Millisecond},
		{"cap5", 10 * time.Second, 75 * time.Second, 85 * time.Second},
		{"overCap5", 10 * time.Second, 74999 * time.Millisecond, 90 * time.Second},
		{"no80sFallback", 10 * time.Second, time.Second, 90 * time.Second},
	} {
		t.Run(ca.name, func(t *testing.T) {
			a := &formatFMP4Track{
				initTrack: &fmp4.InitTrack{
					TimeScale: 8000,
					Codec: &mcodecs.LPCM{
						SampleRate:   8000,
						BitDepth:     16,
						ChannelCount: 1,
					},
				},
				nextSample: &formatFMP4Sample{
					dts: int64(ca.audioDTS/time.Second) * 8000,
					ntp: ntp.Add(-ca.age),
				},
				nextSampleWritten: true,
				startInitialized:  true,
			}
			_, d := nextSegmentStartingPos([]*formatFMP4Track{video, a}, video)
			require.Equal(t, ca.want, d)
			if ca.name == "lag1.5" {
				video.nextSample.dts += 30 * 90000
				video.nextSample.ntp = video.nextSample.ntp.Add(30 * time.Second)
				_, advanced := nextSegmentStartingPos([]*formatFMP4Track{video, a}, video)
				require.Equal(t, ca.want+30*time.Second, advanced)
				video.nextSample.dts -= 30 * 90000
				video.nextSample.ntp = ntp
			}
		})
	}
}

func TestRecorderLPCMFlushRejectsDrift(t *testing.T) {
	f, a, v := startupFixture(t, true)
	f.ri.maxPartSize = 50 * 1024 * 1024
	f.ri.segmentDuration = 2 * time.Second
	ntp := time.Unix(1700000000, 0)
	require.NoError(t, a.write(pcmPauseSample(50*8000, ntp, 0)))
	require.NoError(t, v.write(&formatFMP4Sample{Sample: &fmp4.Sample{Payload: []byte{1}}, dts: 50 * 90000, ntp: ntp}))
	require.NoError(t, a.write(pcmPauseSample(50*8000+320, ntp.Add(40*time.Millisecond), 1)))
	require.NoError(t,
		v.write(&formatFMP4Sample{
			Sample: &fmp4.Sample{
				Payload: []byte{
					1,
				},
				IsNonSyncSample: true,
			},
			dts: 50*90000 + 3600,
			ntp: ntp.Add(40 * time.Millisecond),
		}))
	require.NoError(t, a.write(pcmPauseSample(150*8000, ntp.Add(80*time.Millisecond), 2)))
	require.ErrorContains(t, a.flushPendingLPCM(), "detected drift")
	require.False(t, a.nextSampleWritten)
	require.Less(t, f.currentSegment.endDTS, 51*time.Second)
}
