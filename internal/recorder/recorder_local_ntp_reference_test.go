package recorder

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderLocalNTPReferenceFlips(t *testing.T) {
	for _, local := range []bool{true, false} {
		t.Run(fmt.Sprintf("local=%v", local), func(t *testing.T) {
			dir := clockEvidenceDir(t)
			ntp := time.Unix(1700000000, 0)
			receipt := ntp
			late := 0
			created := map[string]bool{}
			f := &formatFMP4{
				now: func() time.Time {
					return receipt
				},
				ri: &recorderInstance{
					stream: &stream.Stream{
						ReplaceNTP: local,
					},
					streamID: uuid.New(),
					pathFormat2: filepath.Join(dir,
						"%Y-%m-%d_%H-%M-%S-%f.mp4"),
					location:        time.UTC,
					partDuration:    time.Second,
					maxPartSize:     50 * 1024 * 1024,
					segmentDuration: time.Minute,
					parent: test.Logger(func(_ logger.Level,
						format string,
						_ ...any,
					) {
						if format == "sample of track %d received too late, discarding" {
							late++
						}
					}),
					onSegmentCreate: func(path string) {
						require.False(t,
							created[path])
						created[path] = true
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
			f.tracks = []*formatFMP4Track{v, a}
			const count = 7000
			for i := range count {
				at := time.Duration(i) * 20 * time.Millisecond
				receipt = ntp.Add(at)
				if i%2 == 0 {
					require.NoError(t,
						v.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload: clockPayload(uint32(i/2),
									4),
								IsNonSyncSample: i%50 != 0,
							},
							dts: 50*90000 + int64(i)*1800,
							ntp: receipt,
						}))
				}
				if i < 1000 || i >= 3500 {
					require.NoError(t,
						a.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload: clockPayload(uint32(i),
									320),
							},
							dts: 42*8000 + int64(i)*160,
							ntp: receipt,
						}))
				}
			}
			f.close()
			paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
			require.NoError(t, err)
			require.Len(t, paths, 3)
			analysis := analyzeClockRecording(t, paths, 42*time.Second, 160)
			seenA, seenV := analysis.audioIDs, analysis.videoIDs
			metas := analysis.metas

			sort.Slice(metas, func(i, j int) bool { return metas[i].SegmentNumber < metas[j].SegmentNumber })
			maps := make([]time.Duration, 0, len(metas))
			ids := map[[16]byte]bool{}
			for _, m := range metas {
				maps = append(maps, time.Duration(m.NTP)-time.Duration(m.DTS))
				ids[m.StreamID] = true
			}
			require.Equal(t, -8*time.Second, maps[1]-maps[0])
			require.Equal(t, 8*time.Second, maps[2]-maps[1])
			if local {
				require.Len(t, ids, 3)
			} else {
				require.Len(t, ids, 1, "external reference policy unchanged")
			}
			require.Len(t, seenA, 4499)
			require.Len(t, seenV, 3499)
			require.Zero(t, late)
			t.Logf("mapping flips=%v,%v UUIDs=%d audio=%d video=%d late=%d; paused projection "+
				"rejected 8s offset without widening 5s cap",
				maps[1]-maps[0], maps[2]-maps[1], len(ids), len(seenA), len(seenV), late)
		})
	}
}
