package recorder

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/ntpestimator"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderLocalNTPRepeatedCorrections100Minutes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := clockEvidenceDir(t)
		created := map[string]bool{}
		late := 0
		f := &formatFMP4{ri: &recorderInstance{
			stream: &stream.Stream{ReplaceNTP: true}, streamID: uuid.New(),
			pathFormat2: filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f.mp4"),
			location:    time.UTC, partDuration: time.Second, maxPartSize: 50 * 1024 * 1024,
			segmentDuration: time.Minute,
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
		ea, ev := &ntpestimator.Estimator{ClockRate: 8000}, &ntpestimator.Estimator{ClockRate: 90000}
		start := time.Now()
		nextA, nextV := time.Duration(0), time.Duration(0)
		countA, countV, stepsA, stepsV, recoveries := 0, 0, 0, 0, 0
		var prevAN, prevVN time.Time
		var prevAP, prevVP int64
		maxLookback := time.Duration(0)
		for min(nextA, nextV) < 100*time.Minute+time.Second {
			at := min(nextA, nextV)
			if wait := at - time.Since(start); wait > 0 {
				time.Sleep(wait)
			}
			if nextV <= nextA {
				pts := int64(50*90000 + countV*3600)
				ntp := ev.Estimate(pts)
				if !prevVN.IsZero() && (ntp.Sub(prevVN)-timestampToDuration(pts-prevVP, 90000)).Abs() > time.Second {
					stepsV++
					t.Logf("video local NTP re-anchor %d at %v", stepsV, time.Since(start))
				}
				prevVN, prevVP = ntp, pts
				oldAnchor, initialized, oldSegment := v.startNTP, v.startInitialized, f.currentSegment
				require.NoError(t, v.write(&formatFMP4Sample{
					Sample: &fmp4.Sample{Payload: clockPayload(uint32(countV), 4), IsNonSyncSample: countV%25 != 0},
					dts:    pts, ntp: ntp,
				}))
				if initialized && oldAnchor != v.startNTP {
					recoveries++
				}
				if f.currentSegment != oldSegment {
					maxLookback = max(maxLookback, timestampToDuration(v.nextSample.dts, 90000)-f.currentSegment.startDTS)
				}
				countV++
				nextV = time.Duration(float64(countV) * float64(40*time.Millisecond) / (1 - 2700e-6))
			} else {
				// Each packet contains exactly 1000 PCM samples and advances RTP
				// by exactly 1000 ticks. Only physical delivery cadence is slow.
				pts := int64(50*8000 - 12000 + countA*1000)
				ntp := ea.Estimate(pts)
				if !prevAN.IsZero() && (ntp.Sub(prevAN)-timestampToDuration(pts-prevAP, 8000)).Abs() > time.Second {
					stepsA++
					t.Logf("audio local NTP re-anchor %d at %v", stepsA, time.Since(start))
				}
				prevAN, prevAP = ntp, pts
				oldAnchor, initialized := a.startNTP, a.startInitialized
				require.NoError(t, a.write(&formatFMP4Sample{
					Sample: &fmp4.Sample{Payload: clockPayload(uint32(countA), 2000)}, dts: pts, ntp: ntp,
				}))
				if initialized && oldAnchor != a.startNTP {
					recoveries++
				}
				countA++
				nextA = time.Duration(float64(countA) * float64(125*time.Millisecond) / (1 - 2600e-6))
			}
		}
		f.close()
		paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
		require.NoError(t, err)
		seenA, seenV := map[uint32]bool{}, map[uint32]bool{}
		metas := make([]recordstore.Mtxi, 0, len(paths))
		for _, path := range paths {
			data, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			var init fmp4.Init
			require.NoError(t, init.Unmarshal(bytes.NewReader(data)))
			meta := *init.UserData[0].(*recordstore.Mtxi)
			metas = append(metas, meta)
			var parts fmp4.Parts
			require.NoError(t, parts.Unmarshal(data))
			firstVideo := true
			for _, part := range parts {
				for _, track := range part.Tracks {
					rate := int64(8000)
					if track.ID == 1 {
						rate = 90000
					}
					// Exact inverse for these original sample grids, including
					// fractional cross-track container bases.
					ticks := int64(track.BaseTime) + (meta.DTS*rate+int64(time.Second)-1)/int64(time.Second)
					for _, sample := range track.Samples {
						id := binary.BigEndian.Uint32(sample.Payload)
						if track.ID == 2 {
							require.False(t, seenA[id], "duplicate audio")
							seenA[id] = true
							require.True(t, bytes.Equal(clockPayload(id, 2000), sample.Payload))
							require.Equal(t, uint32(1000), sample.Duration)
							require.Equal(t, int64(50*8000-12000)+int64(id)*1000, ticks)
						} else {
							require.False(t, seenV[id], "duplicate video")
							seenV[id] = true
							require.True(t, bytes.Equal(clockPayload(id, 4), sample.Payload))
							require.Equal(t, uint32(3600), sample.Duration)
							require.Equal(t, int64(50*90000)+int64(id)*3600, ticks)
							if firstVideo {
								require.False(t, sample.IsNonSyncSample)
								firstVideo = false
							}
						}
						ticks += int64(sample.Duration)
					}
				}
			}
		}
		sort.Slice(metas, func(i, j int) bool { return metas[i].SegmentNumber < metas[j].SegmentNumber })
		ids := map[[16]byte]bool{}
		var first recordstore.Mtxi
		for i, meta := range metas {
			require.Equal(t, uint64(i), meta.SegmentNumber)
			if i == 0 || meta.StreamID != metas[i-1].StreamID {
				require.False(t, ids[meta.StreamID], "UUID must not reappear in a later range")
				ids[meta.StreamID] = true
				first = meta
			}
			drift := time.Duration(meta.NTP-first.NTP) - time.Duration(meta.DTS-first.DTS)
			require.LessOrEqual(t, drift.Abs(), 5*time.Second)
		}
		require.Len(t, seenA, countA-1)
		require.Len(t, seenV, countV-1)
		require.Zero(t, late)
		require.GreaterOrEqual(t, stepsA, 3)
		require.GreaterOrEqual(t, stepsV, 3)
		require.Equal(t, stepsA+stepsV, recoveries)
		require.GreaterOrEqual(t, len(ids), 4)
		require.LessOrEqual(t, maxLookback, 5*time.Second)
		t.Logf("100m1s 125ms/1000-sample packets: audio=%d/%d video=%d/%d files=%d "+
			"reanchors=%d+%d recoveries=%d UUIDs=%d late=%d maxLookback=%v",
			len(seenA), countA-1, len(seenV), countV-1, len(paths), stepsA, stepsV, recoveries, len(ids), late, maxLookback)
	})
}
