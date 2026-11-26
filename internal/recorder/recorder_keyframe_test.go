package recorder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// recordInterleaved records 4 s of interleaved video (one IDR per second) and audio whose
// timestamps run audioLead ahead of the video for the same arrival, the way the global RTP
// decoder anchors a track that a recorder (NVR) delivers later than the other. It returns the
// "received too late" warnings and, per segment in file order, the video track's samples.
func recordInterleaved(t *testing.T, audioLead time.Duration) ([]string, [][]*fmp4.Sample, []string) {
	t.Helper()
	desc := &description.Session{Medias: []*description.Media{
		{Type: description.MediaTypeVideo, Formats: []rtspformat.Format{test.FormatH264}},
		{Type: description.MediaTypeAudio, Formats: []rtspformat.Format{test.FormatMPEG4Audio}},
	}}

	strm := &stream.Stream{OrigDesc: desc, WriteQueueSize: 4096, RTPMaxPayloadSize: 1450, Parent: test.NilLogger}
	require.NoError(t, strm.Initialize())
	defer strm.Close()

	subStream := &stream.SubStream{Stream: strm, UseRTPPackets: false}
	require.NoError(t, subStream.Initialize())

	dir := t.TempDir()

	var mutex sync.Mutex
	var late []string

	w := &Recorder{
		location:        time.UTC,
		PathFormat:      filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     50 * 1024 * 1024,
		SegmentDuration: 1 * time.Second,
		PathName:        "mypath",
		Stream:          strm,
		Parent: test.Logger(func(_ logger.Level, format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			if msg == "[recorder] sample of track 1 received too late, discarding" ||
				msg == "[recorder] sample of track 2 received too late, discarding" {
				mutex.Lock()
				late = append(late, msg)
				mutex.Unlock()
			}
		}),
	}
	w.Initialize()

	base := 50 * time.Second
	ntp := time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC)
	step := 40 * time.Millisecond

	for i := range 100 {
		at := time.Duration(i) * step

		payload := unit.PayloadH264{{1}} // non-IDR
		if i%25 == 0 {
			payload = unit.PayloadH264{{5}} // IDR once per second
		}
		subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS:     int64(base+at) * 90000 / int64(time.Second),
			NTP:     ntp.Add(at),
			Payload: payload,
		})

		subStream.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
			PTS:     int64(base+at+audioLead) * 44100 / int64(time.Second),
			NTP:     ntp.Add(at),
			Payload: unit.PayloadMPEG4Audio{{1, 2}},
		})
	}

	time.Sleep(200 * time.Millisecond)
	w.Close()

	entries, err := os.ReadDir(filepath.Join(dir, "mypath"))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	video := make([][]*fmp4.Sample, 0, len(names))
	for _, name := range names {
		byts, readErr := os.ReadFile(filepath.Join(dir, "mypath", name))
		require.NoError(t, readErr)
		var parts fmp4.Parts
		require.NoError(t, parts.Unmarshal(byts))
		var samples []*fmp4.Sample
		for _, part := range parts {
			for _, track := range part.Tracks {
				if track.ID == 1 {
					samples = append(samples, track.Samples...)
				}
			}
		}
		video = append(video, samples)
	}

	return late, video, names
}

// With audio decoding 1.5 s ahead of video, at every segment rotation
// the next segment started at the audio's next sample, more than 1 s (maxBasetime) ahead of the
// video's IDR, so the IDR and the following 1.1-1.5 s of video were discarded as "received too
// late" and every segment began on a non-keyframe.
func TestRecorderFMP4SegmentSwitchKeepsKeyframeWhenAudioLeads(t *testing.T) {
	late, video, names := recordInterleaved(t, 1500*time.Millisecond)

	require.Empty(t, late)
	require.GreaterOrEqual(t, len(video), 3, "segments %v", names)

	total := 0
	for i, samples := range video {
		require.NotEmpty(t, samples, "segment %s has no video", names[i])
		require.False(t, samples[0].IsNonSyncSample, "segment %s starts on a non-keyframe", names[i])
		total += len(samples)
	}
	// 100 units written; the recorder holds the newest one back as its next sample.
	require.Equal(t, 99, total)
}

// The unchanged case: audio within maxBasetime of the video still starts the segment at the
// older of the two samples, and nothing is discarded.
func TestRecorderFMP4SegmentSwitchSmallAudioLead(t *testing.T) {
	late, video, _ := recordInterleaved(t, 500*time.Millisecond)

	require.Empty(t, late)
	total := 0
	for _, samples := range video {
		require.False(t, samples[0].IsNonSyncSample)
		total += len(samples)
	}
	require.Equal(t, 99, total)
}
