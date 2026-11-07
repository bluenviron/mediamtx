package recorder

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

// Silence at the source does not justify discarding the last packet it sent.
// Its duration is known from LPCM payload even before the next packet arrives.
func TestRecorderLPCMStallRetainsLastReceivedSample(t *testing.T) {
	f, a, v := startupFixture(t, true)
	f.ri.maxPartSize = 50 * 1024 * 1024
	f.ri.segmentDuration = 2 * time.Second
	ntp := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	for i := range 500 {
		at := time.Duration(i) * 40 * time.Millisecond
		if i < 50 || i >= 150 {
			payload := make([]byte, 632) // actual source carries 316 samples per 320-tick slot
			binary.BigEndian.PutUint32(payload, uint32(i))
			require.NoError(t,
				a.write(&formatFMP4Sample{
					Sample: &fmp4.Sample{
						Payload: payload,
					},
					dts: 50*8000 + int64(i)*320,
					ntp: ntp.Add(at),
				}))
		}
		require.NoError(t,
			v.write(&formatFMP4Sample{
				Sample: &fmp4.Sample{
					Payload: []byte{
						byte(i),
					},
					IsNonSyncSample: i%25 != 0,
				},
				dts: 50*90000 + int64(i)*3600,
				ntp: ntp.Add(at),
			}))
	}
	require.NoError(t, f.currentSegment.close())
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(f.ri.pathFormat2), "*.mp4"))
	require.NoError(t, err)
	seen := make(map[uint32]bool)
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var parts fmp4.Parts
		require.NoError(t, parts.Unmarshal(data))
		var init fmp4.Init
		require.NoError(t, init.Unmarshal(bytes.NewReader(data)))
		meta := init.UserData[0].(*recordstore.Mtxi)
		videoFirst := true
		for _, part := range parts {
			for _, track := range part.Tracks {
				if track.ID == 1 && len(track.Samples) > 0 && videoFirst {
					require.False(t, track.Samples[0].IsNonSyncSample)
					videoFirst = false
				}
				if track.ID == 2 {
					dts := int64(track.BaseTime)
					for _, sample := range track.Samples {
						id := binary.BigEndian.Uint32(sample.Payload)
						require.False(t, seen[id])
						seen[id] = true
						if id == 49 {
							require.Equal(t, uint32(316), sample.Duration)
						} else {
							require.Equal(t, uint32(320), sample.Duration)
						}
						require.InDelta(t,
							int64(50*time.Second+time.Duration(id)*40*time.Millisecond),
							meta.DTS+int64(timestampToDuration(dts,
								8000)),
							125000,
							"audio %d DTS shifted",
							id)
						dts += int64(sample.Duration)
					}
				}
			}
		}
	}
	require.True(t, seen[49], "last received packet before source pause was lost")
	require.Len(t, seen, 399, "all received samples except newest lookahead must survive")
}
