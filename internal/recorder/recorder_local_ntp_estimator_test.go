package recorder

import (
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/ntpestimator"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderLocalNTPEstimatorCorrection(t *testing.T) {
	for _, ca := range []struct {
		name       string
		local      bool
		stationary bool
		lag        time.Duration
		audioGap   bool
	}{
		{"local_advancing", true, false, 0, false},
		{"local_advancing_lag1500", true, false, 1500 * time.Millisecond, false},
		{"local_shared_then_audio_gap", true, false, 1500 * time.Millisecond, true},
		{"absolute_advancing", false, false, 0, false},
		{"local_stationary", true, true, 0, false},
	} {
		t.Run(ca.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				dir := clockEvidenceDir(t)
				created := map[string]bool{}
				late := 0
				f := &formatFMP4{ri: &recorderInstance{
					stream: &stream.Stream{ReplaceNTP: ca.local}, streamID: uuid.New(),
					pathFormat2: filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f.mp4"),
					location:    time.UTC, partDuration: 100 * time.Millisecond,
					maxPartSize: 50 * 1024 * 1024, segmentDuration: 2 * time.Second,
					parent: test.Logger(func(_ logger.Level, format string, _ ...any) {
						if format == "sample of track %d received too late, discarding" {
							late++
						}
					}),
					onSegmentCreate: func(path string) {
						require.False(t, created[path], "filename reused")
						created[path] = true
					},
					onSegmentComplete: func(string, time.Duration) {},
				}}
				a := &formatFMP4Track{
					f: f, id: 2, clockRate: 8000,
					codec: &mcodecs.LPCM{SampleRate: 8000, BitDepth: 16, ChannelCount: 1},
				}
				v := &formatFMP4Track{
					f: f, id: 1, clockRate: 90000,
					codec: &mcodecs.H264{SPS: test.FormatH264.SPS, PPS: test.FormatH264.PPS},
				}
				a.initialize()
				v.initialize()
				f.tracks = []*formatFMP4Track{v, a}
				if ca.stationary {
					f.tracks = []*formatFMP4Track{a}
				}
				ea, ev := &ntpestimator.Estimator{ClockRate: 8000}, &ntpestimator.Estimator{ClockRate: 90000}
				start := time.Now()
				var gotErr error
				var errAt time.Duration
				for tick := time.Duration(0); tick <= 19900*time.Millisecond; tick += 5 * time.Millisecond {
					if tick == 4*time.Second {
						// A host/source delivery hiatus: media clocks advance normally
						// on their next packets, while actual local Estimate re-anchors.
						time.Sleep(8 * time.Second)
					}
					if !ca.stationary && tick%(40*time.Millisecond) == 0 {
						id := uint32(tick / (40 * time.Millisecond))
						pts := int64(50*90000) + int64(id)*3600
						gotErr = v.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{Payload: clockPayload(id, 4), IsNonSyncSample: id%25 != 0},
							dts:    pts, ntp: ev.Estimate(pts),
						})
					}
					if gotErr == nil && tick%(125*time.Millisecond) == 0 &&
						(!ca.audioGap || tick < 4*time.Second || tick >= 7*time.Second) {
						id := uint32(tick / (125 * time.Millisecond))
						pts := int64(50*8000) - int64(ca.lag/time.Millisecond)*8 + int64(id)*1000
						if ca.stationary {
							pts = 50 * 8000
						}
						gotErr = a.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{Payload: clockPayload(id, 2000)},
							dts:    pts, ntp: ea.Estimate(pts),
						})
					}
					if gotErr != nil {
						errAt = time.Since(start)
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				f.close()
				if !ca.local || ca.stationary {
					require.ErrorContains(t, gotErr, "detected drift between recording duration and absolute time")
					t.Logf("strict control reset at %v", errAt)
					return
				}
				require.NoError(t, gotErr, "an advancing local clock must survive the 8s Estimate correction")
				paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
				require.NoError(t, err)
				analysis := analyzeClockRecording(t, paths, 50*time.Second-ca.lag, 1000)
				seenA, seenV := analysis.audioIDs, analysis.videoIDs
				ids := analysis.streamIDs

				wantAudio := 159
				if ca.audioGap {
					wantAudio -= 24
				}
				require.Len(t, seenA, wantAudio)
				require.Len(t, seenV, 497)
				require.Zero(t, late)
				require.GreaterOrEqual(t, len(ids), 2)
				t.Logf("8s local Estimate correction: audio=%d video=%d late=%d files=%d UUIDranges=%d",
					len(seenA), len(seenV), late, len(paths), len(ids))
			})
		})
	}
}
