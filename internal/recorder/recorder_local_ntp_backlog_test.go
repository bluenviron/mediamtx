package recorder

import (
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestRecorderReaderBacklogPreservesIngressFreshness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		desc := &description.Session{Medias: []*description.Media{
			{Type: description.MediaTypeVideo, Formats: []rtspformat.Format{test.FormatH264}},
			{Type: description.MediaTypeAudio, Formats: []rtspformat.Format{&rtspformat.LPCM{
				PayloadTyp: 96, BitDepth: 16, SampleRate: 8000, ChannelCount: 1,
			}}},
		}}
		strm := &stream.Stream{
			OrigDesc: desc, ReplaceNTP: true, WriteQueueSize: 1024,
			RTPMaxPayloadSize: 1450, Parent: test.NilLogger,
		}
		require.NoError(t, strm.Initialize())
		defer strm.Close()
		ss := &stream.SubStream{Stream: strm, UseRTPPackets: false}
		require.NoError(t, ss.Initialize())
		r := &stream.Reader{SkipOutboundBytes: true, Parent: test.NilLogger, TimeNow: time.Now}
		start := time.Now()
		paused, release, drained := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() {
			<-paused
			time.Sleep(8 * time.Second)
			close(release)
		}()
		dir := clockEvidenceDir(t)
		created := map[string]bool{}
		duplicates, late := 0, 0
		var queuedStamp, consumedStamp time.Duration
		f := &formatFMP4{ri: &recorderInstance{
			stream: strm, reader: r,
			pathFormat2: filepath.Join(dir, "%Y-%m-%d_%H-%M-%S-%f.mp4"),
			location:    time.UTC, partDuration: 100 * time.Millisecond,
			maxPartSize: 50 * 1024 * 1024, segmentDuration: time.Second,
			parent: test.Logger(func(_ logger.Level, format string, _ ...any) {
				if format == "sample of track %d received too late, discarding" {
					late++
				}
			}),
			onSegmentCreate: func(path string) {
				if created[path] {
					duplicates++
				}
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
		r.OnData(desc.Medias[0], desc.Medias[0].Formats[0], func(u *unit.Unit) error {
			id := uint32((u.PTS - 50*90000) / 3600)
			if id == 25 {
				queuedStamp, consumedStamp = r.ReceivedAt().Sub(start), time.Since(start)
			}
			if err := v.write(&formatFMP4Sample{
				Sample: &fmp4.Sample{Payload: clockPayload(id, 4), IsNonSyncSample: id%25 != 0},
				dts:    u.PTS, ntp: u.NTP,
			}); err != nil {
				return err
			}
			if id == 24 {
				close(paused)
				<-release // Model slow disk/codec work before the next queued video callback.
			}
			if id == 497 {
				close(drained)
			}
			return nil
		})
		r.OnData(desc.Medias[1], desc.Medias[1].Formats[0], func(u *unit.Unit) error {
			id := uint32((u.PTS - 42*8000) / 1000)
			return a.write(&formatFMP4Sample{
				Sample: &fmp4.Sample{Payload: clockPayload(id, 2000)}, dts: u.PTS, ntp: u.NTP,
			})
		})
		strm.AddReader(r)
		for tick := time.Duration(0); tick <= 19900*time.Millisecond; tick += 5 * time.Millisecond {
			if tick%(40*time.Millisecond) == 0 {
				ss.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
					PTS: 50*90000 + int64(tick/time.Millisecond)*90,
				})
			}
			if tick%(125*time.Millisecond) == 0 {
				ss.WriteUnit(desc.Medias[1], desc.Medias[1].Formats[0], &unit.Unit{
					PTS: 42*8000 + int64(tick/time.Millisecond)*8,
				})
			}
			time.Sleep(5 * time.Millisecond)
		}
		select {
		case <-drained:
		case err := <-r.Error():
			t.Fatalf("reader failed: %v", err)
		}
		require.Zero(t, r.OutboundFramesDiscarded())
		strm.RemoveReader(r)
		f.close()
		require.Equal(t, time.Second, queuedStamp)
		require.Greater(t, consumedStamp-queuedStamp, 7*time.Second)
		paths, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
		require.NoError(t, err)
		analysis := analyzeClockRecording(t, paths, 42*time.Second, 1000)
		seenA, seenV := analysis.audioIDs, analysis.videoIDs

		t.Logf("video enqueued at=%v consumed at=%v files=%d audio=%d/159 video=%d/497 late=%d duplicateNames=%d",
			queuedStamp, consumedStamp, len(paths), len(seenA), len(seenV), late, duplicates)
		require.Len(t, seenA, 159)
		require.Len(t, seenV, 497)
		require.Zero(t, late)
		require.Zero(t, duplicates)
	})
}
