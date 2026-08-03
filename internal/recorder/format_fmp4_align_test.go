package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
)

// These tests drive the segment directly rather than a whole stream.
//
// What changed is where a part ends, and that decision is made from a sample's
// flags and the part's duration and size — nothing else. Feeding it a stream
// would mean synthesising a decodable H.264 bitstream, and the test would then
// be exercising the codec parsers rather than the boundary rule.

// alignFixture is a segment writing to a real file, which is what
// closeCurPart needs.
type alignFixture struct {
	segment *formatFMP4Segment
	video   *formatFMP4Track
	audio   *formatFMP4Track
	path    string
}

func newAlignFixture(t *testing.T, align bool, partDuration time.Duration,
	maxPartSize conf.StringSize, withAudio bool,
) *alignFixture {
	t.Helper()

	instance := &recorderInstance{
		partDuration:        partDuration,
		partAlignToKeyframe: align,
		maxPartSize:         maxPartSize,
		parent:              &Recorder{Parent: test.NilLogger},
	}

	format := &formatFMP4{ri: instance}
	video := &formatFMP4Track{
		f: format, id: 1, clockRate: 90000,
		initTrack: &fmp4.InitTrack{
			ID: 1, TimeScale: 90000,
			Codec: &mcodecs.H264{SPS: test.FormatH264.SPS, PPS: test.FormatH264.PPS},
		},
	}
	format.tracks = []*formatFMP4Track{video}
	format.hasVideo = true

	var audio *formatFMP4Track
	if withAudio {
		audio = &formatFMP4Track{
			f: format, id: 2, clockRate: 44100,
			initTrack: &fmp4.InitTrack{
				ID: 2, TimeScale: 44100,
				Codec: &mcodecs.MPEG4Audio{Config: mpeg4audio.AudioSpecificConfig{
					Type: 2, SampleRate: 44100, ChannelCount: 2,
				}},
			},
		}
		format.tracks = append(format.tracks, audio)
	}

	path := filepath.Join(t.TempDir(), "segment.mp4")
	file, err := os.Create(path) //nolint:gosec // a test file in a temp dir.
	require.NoError(t, err)
	t.Cleanup(func() { file.Close() })

	segment := &formatFMP4Segment{f: format, startDTS: 0, fi: file}
	segment.initialize()

	return &alignFixture{segment: segment, video: video, audio: audio, path: path}
}

// write feeds one sample. payload size is what the size limit is measured in.
func (f *alignFixture) write(
	t *testing.T, track *formatFMP4Track, dts time.Duration, keyframe bool, payload int,
) {
	t.Helper()

	sample := &formatFMP4Sample{
		Sample: &fmp4.Sample{
			Duration:        uint32(40 * int(track.initTrack.TimeScale) / 1000),
			IsNonSyncSample: !keyframe,
			Payload:         make([]byte, payload),
		},
		dts: int64(dts) * int64(track.initTrack.TimeScale) / int64(time.Second),
		ntp: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC).Add(dts),
	}
	require.NoError(t, f.segment.write(track, sample, dts))
}

// parts is how many parts the segment has produced so far, including the open
// one.
func (f *alignFixture) parts() uint32 { return f.segment.nextPartNumber }

// videoStream feeds frames at 40 ms with an IDR every gop frames.
func (f *alignFixture) videoStream(t *testing.T, frames, gop, payload int) {
	t.Helper()

	for i := range frames {
		f.write(t, f.video, time.Duration(i)*40*time.Millisecond, i%gop == 0, payload)
	}
}

// Without the option a part ends as soon as it has lasted long enough, wherever
// that falls. This is the behaviour every existing deployment relies on, and the
// option must not change it.
func TestPartsAreNotAlignedByDefault(t *testing.T) {
	f := newAlignFixture(t, false, 200*time.Millisecond, 50*1024*1024, false)

	// 25 frames of 40 ms is one second, with a keyframe only at the start.
	f.videoStream(t, 25, 25, 100)

	// Five parts of 200 ms, and only the first can start at a keyframe.
	require.Equal(t, uint32(5), f.parts())
}

// With the option a part ends only before a sample decoding can start from, so
// every part is independently decodable. This is the whole purpose.
func TestPartsEndOnlyAtKeyframesWhenAligned(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 50*1024*1024, false)

	// Four seconds at a one-second GOP.
	f.videoStream(t, 100, 25, 100)

	// One part per GOP: the timer wants a cut every 200 ms and gets one every
	// second, when the next keyframe arrives.
	require.Equal(t, uint32(4), f.parts())
}

// The requested duration becomes a minimum rather than a target. A part runs to
// the first random access point at or after it, so its length follows the
// keyframe interval — the cost of alignment, and it has to be visible rather
// than surprising.
func TestPartDurationBecomesAMinimumWhenAligned(t *testing.T) {
	// A GOP of two seconds against a requested part of 200 ms.
	f := newAlignFixture(t, true, 200*time.Millisecond, 50*1024*1024, false)
	f.videoStream(t, 200, 50, 100)

	// Eight seconds at a two-second GOP is four parts, not forty.
	require.Equal(t, uint32(4), f.parts())
}

// A keyframe arriving before the part has lasted long enough does not end it:
// alignment narrows where a part may end, it does not make every keyframe a
// boundary. Otherwise a stream with a short GOP would produce a part per GOP
// regardless of the configured duration.
func TestAKeyframeDoesNotEndAShortPart(t *testing.T) {
	f := newAlignFixture(t, true, 1*time.Second, 50*1024*1024, false)

	// A keyframe every 200 ms against a requested part of one second.
	f.videoStream(t, 100, 5, 100)

	// Four seconds, so four parts — each ending at the first keyframe at or
	// after one second, not at every keyframe.
	require.Equal(t, uint32(4), f.parts())
}

// A stream with no video has no random access points to align to. Refusing to
// cut would mean never cutting: the recording would grow into a single part
// until it hit the size limit.
func TestAudioOnlyStreamsStillCutParts(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 50*1024*1024, true)
	f.segment.f.hasVideo = false

	for i := range 100 {
		f.write(t, f.audio, time.Duration(i)*40*time.Millisecond, false, 100)
	}

	require.Equal(t, uint32(20), f.parts())
}

// An audio sample must not end a part in a stream that has video. Cutting there
// would start the next part with a video sample that is not a random access
// point, which is exactly what alignment exists to prevent.
func TestAudioSamplesDoNotEndAPartInAVideoStream(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 50*1024*1024, true)

	// Audio arriving between video frames, with the video GOP at one second.
	for i := range 100 {
		dts := time.Duration(i) * 40 * time.Millisecond
		f.write(t, f.video, dts, i%25 == 0, 100)
		f.write(t, f.audio, dts, false, 20)
	}

	// Still one part per GOP: the audio samples changed nothing.
	require.Equal(t, uint32(4), f.parts())
}

// A camera that stops emitting keyframes must keep recording. The part is ended
// where the size limit falls, which produces one fragment that does not start
// at a random access point — visible to a reader in the sample flags — and that
// is far better than the recording stopping, which is what returning an error
// here would do.
func TestAStreamWithoutKeyframesKeepsRecording(t *testing.T) {
	f := newAlignFixture(t, true, 200*time.Millisecond, 4096, false)

	// One keyframe at the start and nothing after it, with payloads big enough
	// to reach the limit.
	f.videoStream(t, 100, 1000, 512)

	require.Greater(t, f.parts(), uint32(1),
		"a stream with no keyframes produced one part; the size limit did not end it")
}

// And the same stream without alignment must still fail on the size limit,
// because that is the guard against unbounded memory and this change does not
// remove it.
func TestTheSizeLimitStillFailsWithoutAlignment(t *testing.T) {
	f := newAlignFixture(t, false, 1*time.Hour, 4096, false)

	// A part that can never end on duration, so only the limit can stop it.
	var err error
	for i := range 100 {
		sample := &formatFMP4Sample{
			Sample: &fmp4.Sample{
				Duration:        3600,
				IsNonSyncSample: i != 0,
				Payload:         make([]byte, 512),
			},
			dts: int64(i) * 3600,
			ntp: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		}
		if err = f.segment.write(f.video, sample, time.Duration(i)*40*time.Millisecond); err != nil {
			break
		}
	}

	require.Error(t, err, "the size limit stopped guarding against an unbounded part")
	require.Contains(t, err.Error(), "maximum part size")
}
