package recorder

import (
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/stream"
)

func TestRecorderLocalNTPRejectsMediaJump(t *testing.T) {
	f, a, _ := startupFixture(t, false)
	f.ri.stream = &stream.Stream{ReplaceNTP: true}
	f.ri.maxPartSize = 50 * 1024 * 1024
	f.ri.segmentDuration = 5 * time.Minute
	start := time.Unix(1700000000, 0)
	receipt := start
	f.now = func() time.Time { return receipt }
	write := func(pts int64, elapsed time.Duration) error {
		receipt = start.Add(elapsed)
		return a.write(&formatFMP4Sample{
			Sample: &fmp4.Sample{Payload: clockPayload(uint32(elapsed/time.Millisecond), 2000)},
			dts:    pts, ntp: receipt,
		})
	}
	require.NoError(t, write(50*8000, 0))
	require.NoError(t, write(50*8000+1000, 125*time.Millisecond))
	require.NoError(t, write(150*8000, 250*time.Millisecond))
	require.ErrorContains(t, a.flushPendingLPCM(), "detected drift")
	require.False(t, a.nextSampleWritten)
	require.Less(t, f.currentSegment.endDTS, 51*time.Second)
	require.ErrorContains(t, write(150*8000+1000, 375*time.Millisecond), "detected drift")
}

func TestRecorderLocalNTPCorrectionScope(t *testing.T) {
	for _, local := range []bool{true, false} {
		f := &formatFMP4{ri: &recorderInstance{stream: &stream.Stream{ReplaceNTP: local}}}
		require.Equal(t, local, f.localCorrection(125*time.Millisecond, 125*time.Millisecond))
		require.Equal(t, local, f.localCorrection(125*time.Millisecond, 8125*time.Millisecond))
		require.False(t, f.localCorrection(0, 125*time.Millisecond))
		require.False(t, f.localCorrection(-125*time.Millisecond, 125*time.Millisecond))
		require.False(t, f.localCorrection(100*time.Second, 125*time.Millisecond))
	}
}
