package recorder

import (
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
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

func TestRecorderLocalNTPWallClockSteps(t *testing.T) {
	for _, ca := range []struct {
		name            string
		local           bool
		stationary      bool
		lag             time.Duration
		step            time.Duration
		segmentDuration time.Duration
		collision       bool
	}{
		{"local_forward", true, false, 0, 8 * time.Second, time.Second, false},
		{"local_backward", true, false, 0, -8 * time.Second, 2 * time.Second, false},
		{"local_backward_collision", true, false, 0, -8 * time.Second, time.Second, true},
		{"absolute_forward", false, false, 0, 8 * time.Second, time.Second, false},
		{"absolute_backward", false, false, 0, -8 * time.Second, time.Second, false},
	} {
		t.Run(ca.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				dir := clockEvidenceDir(t)
				created := map[string]bool{}
				completed := map[string][]byte{}
				late := 0
				f := &formatFMP4{ri: &recorderInstance{
					stream: &stream.Stream{ReplaceNTP: ca.local}, streamID: uuid.New(),
					pathFormat2: filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f.mp4"),
					location:    time.UTC, partDuration: 100 * time.Millisecond,
					maxPartSize: 50 * 1024 * 1024, segmentDuration: time.Second,
					parent: test.Logger(func(_ logger.Level, format string, _ ...any) {
						if format == "sample of track %d received too late, discarding" {
							late++
						}
					}),
					onSegmentCreate: func(path string) {
						require.False(t, created[path], "filename reused")
						created[path] = true
					},
					onSegmentComplete: func(path string, _ time.Duration) {
						data, err := os.ReadFile(path)
						require.NoError(t, err)
						completed[path] = data
						archive := filepath.Join(dir, "completed-before-collision")
						require.NoError(t, os.MkdirAll(archive, 0o755))
						require.NoError(t, os.WriteFile(filepath.Join(archive, filepath.Base(path)), data, 0o644))
					},
				}}
				f.ri.segmentDuration = ca.segmentDuration
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
				wallOffset := time.Duration(0)
				wallNow := func() time.Time { return time.Now().Add(wallOffset) }
				ea,
					ev := &ntpestimator.Estimator{
					ClockRate: 8000,
					Now:       wallNow,
				},
					&ntpestimator.Estimator{
						ClockRate: 90000,
						Now:       wallNow,
					}
				start := time.Now()
				type timingEvent struct {
					Track   int
					ID      uint32
					DTS     int64
					NTP     int64
					Receipt int64
					Bytes   int
					Sync    bool
				}
				events := []timingEvent{}
				var gotErr error
				var errAt time.Duration
				for tick := time.Duration(0); tick <= 19900*time.Millisecond; tick += 5 * time.Millisecond {
					if tick == 12*time.Second {
						// Change only wall time passed to the real Estimator algorithm.
						// Monotonic ingress and continuous media cadence do not jump.
						wallOffset = ca.step
					}
					if !ca.stationary && tick%(40*time.Millisecond) == 0 {
						id := uint32(tick / (40 * time.Millisecond))
						pts := int64(50*90000) + int64(id)*3600
						ntp := ev.Estimate(pts)
						events = append(events, timingEvent{1, id, pts, int64(ntp.Sub(start)), int64(time.Since(start)), 4, id%25 == 0})
						gotErr = v.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{Payload: clockPayload(id, 4), IsNonSyncSample: id%25 != 0},
							dts:    pts, ntp: ntp,
						})
					}
					if gotErr == nil && tick%(125*time.Millisecond) == 0 {
						id := uint32(tick / (125 * time.Millisecond))
						pts := int64(50*8000) - int64(ca.lag/time.Millisecond)*8 + int64(id)*1000
						if ca.stationary {
							pts = 50 * 8000
						}
						ntp := ea.Estimate(pts)
						events = append(events, timingEvent{2, id, pts, int64(ntp.Sub(start)), int64(time.Since(start)), 2000, true})
						gotErr = a.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{Payload: clockPayload(id, 2000)},
							dts:    pts, ntp: ntp,
						})
					}
					if gotErr != nil {
						errAt = time.Since(start)
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				eventData, marshalErr := json.MarshalIndent(map[string]any{
					"scope":     "synthetic recorder-unit inputs; not camera/wire payload or historical inputs",
					"units":     "DTS track ticks; NTP relative wall nanoseconds; Receipt monotonic nanoseconds",
					"local_ntp": ca.local, "wall_step_ns": int64(ca.step), "events": events,
				}, "", "  ")
				require.NoError(t, marshalErr)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "input-events.json"), eventData, 0o644))
				f.close()
				if ca.collision {
					require.ErrorIs(t, gotErr, fs.ErrExist, "canonical collision must fail closed")
					require.Contains(t, completed, f.currentSegment.path)
					for path, before := range completed {
						after, readErr := os.ReadFile(path)
						require.NoError(t, readErr)
						require.Equal(t, before, after)
						require.Equal(t, sha256.Sum256(before), sha256.Sum256(after))
					}
					before := completed[f.currentSegment.path]
					t.Logf("safe canonical ErrExist at %v: %s oldBytes=%d oldSHA256=%x; all completed files unchanged",
						errAt, filepath.Base(f.currentSegment.path), len(before), sha256.Sum256(before))
					return
				}
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

				require.Len(t, seenA, 159)
				require.Len(t, seenV, 497)
				require.Zero(t, late)
				require.GreaterOrEqual(t, len(ids), 2)
				t.Logf("true wall step: audio=%d video=%d late=%d files=%d UUIDranges=%d",
					len(seenA), len(seenV), late, len(paths), len(ids))
			})
		})
	}
}
