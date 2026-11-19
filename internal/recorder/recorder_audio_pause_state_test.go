package recorder

import (
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"
)

func pcmPauseSample(dts int64, ntp time.Time, id byte) *formatFMP4Sample {
	p := make([]byte, 640)
	p[0] = id
	return &formatFMP4Sample{Sample: &fmp4.Sample{Payload: p}, dts: dts, ntp: ntp}
}

func TestRecorderLPCMFlushStateAndWriteError(t *testing.T) {
	f, a, v := startupFixture(t, true)
	f.ri.maxPartSize = 50 * 1024 * 1024
	ntp := time.Unix(1700000000, 0)
	for i := range 3 {
		require.NoError(t,
			a.write(pcmPauseSample(50*8000+int64(i)*320,
				ntp.Add(time.Duration(i)*40*time.Millisecond),
				byte(i))))
		require.NoError(t,
			v.write(&formatFMP4Sample{
				Sample: &fmp4.Sample{
					Payload: []byte{
						1,
					},
					IsNonSyncSample: i%25 != 0,
				},
				dts: 50*90000 + int64(i)*3600,
				ntp: ntp.Add(time.Duration(i) * 40 * time.Millisecond),
			}))
	}
	part := f.currentSegment.curPart
	count := len(part.partTracks[a].Samples)
	part.maxPartSize = 0
	require.Error(t, a.flushPendingLPCM())
	require.False(t, a.nextSampleWritten)
	require.Len(t, part.partTracks[a].Samples, count)
	part.maxPartSize = f.ri.maxPartSize
	require.NoError(t, a.flushPendingLPCM())
	require.True(t, a.nextSampleWritten)
	require.Len(t, part.partTracks[a].Samples, count+1)
	require.NoError(t, a.flushPendingLPCM())
	require.Len(t, part.partTracks[a].Samples, count+1)
	next := pcmPauseSample(54*8000, ntp.Add(4*time.Second), 3)
	require.NoError(t, a.write(next))
	require.False(t, a.nextSampleWritten)
	require.Same(t, next, a.nextSample)
	require.Len(t, part.partTracks[a].Samples, count+1)
	require.NoError(t, a.write(pcmPauseSample(54*8000+320, ntp.Add(4040*time.Millisecond), 4)))
}

func TestRecorderLPCMFlushInitialAnchor(t *testing.T) {
	f, a, v := startupFixture(t, true)
	f.ri.maxPartSize = 50 * 1024 * 1024
	f.ri.segmentDuration = 2 * time.Second
	ntp := time.Unix(1700000000, 0)
	require.NoError(t, a.write(pcmPauseSample(50*8000, ntp, 0)))
	for i := 0; i <= 50; i++ {
		require.NoError(t,
			v.write(&formatFMP4Sample{
				Sample: &fmp4.Sample{
					Payload: []byte{
						1,
					},
					IsNonSyncSample: i%25 != 0,
				},
				dts: 50*90000 + int64(i)*3600,
				ntp: ntp.Add(time.Duration(i) * 40 * time.Millisecond),
			}))
	}
	require.True(t, a.nextSampleWritten, "first held packet should have been committed")
	require.True(t, a.startInitialized, "committing first PCM must establish its drift anchor")
}
