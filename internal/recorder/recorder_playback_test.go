package recorder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/playback"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// Supply NTP directly through a stream and exercise the
// complete recorder lifecycle. Wall-clock corrections do not alter media PTS
// or the synthetic monotonic time used by recorder restart timers.
func TestRecorderClockCorrectionPlayback(t *testing.T) {
	for _, jump := range []time.Duration{
		0, 5100 * time.Millisecond, 24 * time.Hour, -24 * time.Hour,
	} {
		t.Run(jump.String(), func(t *testing.T) {
			dir := t.TempDir()
			start := time.Date(2026, 1, 2, 12, 0, 0, 0, time.Local)
			recordPath := filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f")
			synctest.Test(t, func(t *testing.T) {
				desc := &description.Session{Medias: []*description.Media{{
					Type:    description.MediaTypeVideo,
					Formats: []rtspformat.Format{test.FormatH264},
				}}}
				strm := &stream.Stream{OrigDesc: desc, WriteQueueSize: 512, RTPMaxPayloadSize: 1450, Parent: test.NilLogger}
				require.NoError(t, strm.Initialize())
				defer strm.Close()
				// Keep the initialized stream formats in pass-through mode so
				// these explicitly supplied timestamps reach the recorder.
				// Enable only the recorder's locally-estimated-NTP policy.
				strm.ReplaceNTP = true
				sub := &stream.SubStream{Stream: strm}
				require.NoError(t, sub.Initialize())
				r := &Recorder{
					PathFormat: recordPath, Format: conf.RecordFormatFMP4,
					PartDuration: time.Second, SegmentDuration: 4 * time.Second, MaxPartSize: 1 << 20,
					PathName: "clock", Stream: strm, Parent: test.NilLogger,
				}
				r.Initialize()
				for i := 0; i <= 80; i++ {
					ntp := start.Add(time.Duration(i) * 250 * time.Millisecond)
					if i >= 19 {
						ntp = ntp.Add(jump)
					}
					typ := byte(1)
					if i%4 == 0 {
						typ = 5
					}
					sub.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
						PTS: int64(i) * 22500, NTP: ntp,
						Payload: unit.PayloadH264{test.FormatH264.SPS, test.FormatH264.PPS, {typ, byte(i)}},
					})
					synctest.Wait()
					time.Sleep(250 * time.Millisecond)
					synctest.Wait()
				}
				r.Close()
			})
			pc := &conf.Path{Name: "clock", RecordPath: recordPath, RecordFormat: conf.RecordFormatFMP4}
			segments, err := recordstore.FindSegments(pc, "clock", nil, nil)
			require.NoError(t, err)
			recorded := make([]byte, 0, 80)
			identities := make(map[bool][16]byte)
			for _, seg := range segments {
				b, e := os.ReadFile(seg.Fpath)
				require.NoError(t, e)
				var init fmp4.Init
				require.NoError(t, init.Unmarshal(bytes.NewReader(b)))
				var parts fmp4.Parts
				require.NoError(t, parts.Unmarshal(b))
				ids := []byte{}
				for _, part := range parts {
					for _, track := range part.Tracks {
						for _, sample := range track.Samples {
							ids = append(ids, sample.Payload[len(sample.Payload)-1])
							require.Equal(t, uint32(22500), sample.Duration)
							require.Zero(t, sample.PTSOffset)
						}
					}
				}
				meta := init.UserData[0].(*recordstore.Mtxi)
				require.NotEmpty(t, ids)
				require.Equal(t, time.Duration(ids[0])*250*time.Millisecond, time.Duration(meta.DTS))
				corrected := jump != 0 && ids[0] >= 20
				expectedStart := start.Add(time.Duration(ids[0]) * 250 * time.Millisecond)
				if corrected {
					expectedStart = expectedStart.Add(jump)
				}
				require.True(t, expectedStart.Equal(seg.Start))
				require.Equal(t, expectedStart.UnixNano(), meta.NTP)
				if id, ok := identities[corrected]; ok {
					require.Equal(t, id, meta.StreamID)
				} else {
					identities[corrected] = meta.StreamID
				}
				recorded = append(recorded, ids...)
			}
			slices.Sort(recorded)
			expectedIDs := func(first, end int) []byte {
				ids := make([]byte, end-first)
				for i := range ids {
					ids[i] = byte(first + i)
				}
				return ids
			}
			require.Equal(t, expectedIDs(0, 80), recorded) // final lookahead is excluded
			s := &playback.Server{
				Address:      "127.0.0.1:19996",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				PathConfs:    map[string]*conf.Path{"clock": pc},
				AuthManager:  &test.AuthManager{AuthenticateImpl: func(*auth.Request) (string, *auth.Error) { return "", nil }},
				Parent:       test.NilLogger,
			}
			require.NoError(t, s.Initialize())
			defer s.Close()
			client := &http.Client{Timeout: 10 * time.Second}
			get := func(route string, values url.Values) (int, []byte) {
				res, e := client.Get("http://127.0.0.1:19996/" + route + "?" + values.Encode())
				require.NoError(t, e)
				defer res.Body.Close()
				b, e := io.ReadAll(res.Body)
				require.NoError(t, e)
				return res.StatusCode, b
			}
			status, b := get("list", url.Values{"path": {"clock"}})
			require.Equal(t, 200, status)
			type listRange struct {
				Start    time.Time
				Duration float64
			}
			var listed []listRange
			require.NoError(t, json.Unmarshal(b, &listed))
			expectedRanges := []listRange{{start, 20}}
			if jump != 0 {
				expectedRanges = []listRange{{start, 5}, {start.Add(jump + 5*time.Second), 15}}
				if jump < 0 {
					slices.Reverse(expectedRanges)
				}
			}
			require.Len(t, listed, len(expectedRanges))
			for i, entry := range listed {
				require.True(t, expectedRanges[i].Start.Equal(entry.Start))
				require.Equal(t, expectedRanges[i].Duration, entry.Duration)
			}
			if jump > 0 {
				// A filtered listing exposes the next range at its corrected
				// wall time; callers can start GET at that advertised time.
				status, b = get("list", url.Values{
					"path":  {"clock"},
					"start": {start.Add(8 * time.Second).Format(time.RFC3339Nano)},
					"end":   {start.Add(jump + 13*time.Second).Format(time.RFC3339Nano)},
				})
				require.Equal(t, http.StatusOK, status)
				listed = nil
				require.NoError(t, json.Unmarshal(b, &listed))
				require.Len(t, listed, 1)
				require.True(t, start.Add(jump+5*time.Second).Equal(listed[0].Start))
				require.Equal(t, float64(8), listed[0].Duration)
			}

			for _, q := range []struct {
				name             string
				offset, duration time.Duration
			}{
				{"before", time.Second, time.Second},
				{"after", jump + 8*time.Second, time.Second},
				{"cross", 3 * time.Second, max(jump, 0) + 10*time.Second},
				{"gap", 8 * time.Second, max(jump, 0) + 5*time.Second},
				{"after_rollover", jump + 10*time.Second, 2 * time.Second},
				{"corrected_range", jump + 5*time.Second, 15 * time.Second},
			} {
				values := url.Values{
					"path":     {"clock"},
					"start":    {start.Add(q.offset).Format(time.RFC3339Nano)},
					"duration": {fmt.Sprint(q.duration.Seconds())},
				}
				status, b = get("get", values)
				if q.name == "gap" && jump != 0 {
					// GET does not skip to another recording when its start is
					// in a gap, even if the requested end includes that range.
					require.Equal(t, http.StatusNotFound, status)
					continue
				}
				require.Equal(t, http.StatusOK, status)
				var parts fmp4.Parts
				require.NoError(t, parts.Unmarshal(b))
				ids := []byte{}
				var end uint64
				for _, part := range parts {
					for _, track := range part.Tracks {
						end = track.BaseTime
						for _, sample := range track.Samples {
							ids = append(ids, sample.Payload[len(sample.Payload)-1])
							end += uint64(sample.Duration)
						}
					}
				}
				var want []byte
				switch q.name {
				case "before":
					want = expectedIDs(4, 8)
				case "after":
					want = expectedIDs(32, 36)
				case "cross":
					want = expectedIDs(12, 52)
					if jump != 0 {
						want = expectedIDs(12, 20)
					}
				case "gap":
					want = expectedIDs(32, 52)
				case "after_rollover":
					want = expectedIDs(40, 48)
				case "corrected_range":
					want = expectedIDs(20, 80)
				}
				require.Equal(t, want, ids, q.name)
				require.Equal(t, uint64(len(want))*22500, end, q.name)
			}
		})
	}
}
