package mpegts

import (
	"bufio"
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	srt "github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// fakeSRTConn implements only the SetWriteDeadline method used by the
// FromStream callbacks; all other methods inherit from a nil embedded
// interface and would panic if invoked. This is intentional: the tests below
// only exercise code paths that touch SetWriteDeadline (or do not touch
// sconn at all).
type fakeSRTConn struct {
	srt.Conn
}

func (fakeSRTConn) SetWriteDeadline(_ time.Time) error { return nil }

// extractOnData reaches into the unexported onDatas map of stream.Reader to
// retrieve the callback registered by FromStream. This avoids spinning up a
// full async stream pipeline and keeps the recovery tests focused.
func extractOnData(
	t *testing.T,
	r *stream.Reader,
	medi *description.Media,
	forma format.Format,
) stream.OnDataFunc {
	t.Helper()
	rv := reflect.ValueOf(r).Elem()
	field := rv.FieldByName("onDatas")
	require.True(t, field.IsValid(), "onDatas field not found")
	mapVal := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()

	formats := mapVal.MapIndex(reflect.ValueOf(medi))
	require.True(t, formats.IsValid(), "no callbacks registered for media")
	cb := formats.MapIndex(reflect.ValueOf(forma))
	require.True(t, cb.IsValid(), "no callback registered for format")
	return cb.Interface().(stream.OnDataFunc)
}

func TestFromStreamH264DTSExtractorRecovery(t *testing.T) {
	medi := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH264},
	}
	desc := &description.Session{Medias: []*description.Media{medi}}

	var warnLogs []string
	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...any) {
			if l == logger.Warn {
				warnLogs = append(warnLogs, fmt.Sprintf(format, args...))
			}
		}),
	}

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)

	err := FromStream(desc, r, bw, fakeSRTConn{}, time.Second)
	require.NoError(t, err)

	cb := extractOnData(t, r, medi, medi.Formats[0])

	idr := unit.PayloadH264{test.FormatH264.SPS, test.FormatH264.PPS, {0x65}}

	// 1) Prime the extractor with a valid IDR.
	require.NoError(t, cb(&unit.Unit{PTS: 90000, Payload: idr}))

	// 2) Send a unit whose PTS goes backwards. Before the fix, the DTS
	//    extractor returned an error here that propagated out of the reader
	//    and tore down the SRT receiver. The fix logs a warning, resets the
	//    extractor, and returns nil so the connection survives.
	require.NoError(t, cb(&unit.Unit{PTS: 0, Payload: idr}))

	require.NotEmpty(t, warnLogs, "expected a warn log after the DTS error")
	require.True(t, strings.Contains(warnLogs[0], "H264 DTS extractor reset"),
		"unexpected warn log: %q", warnLogs[0])

	// 3) The next valid IDR should re-prime the extractor and succeed.
	require.NoError(t, cb(&unit.Unit{PTS: 180000, Payload: idr}))
}

func TestFromStreamH265DTSExtractorRecovery(t *testing.T) {
	medi := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{test.FormatH265},
	}
	desc := &description.Session{Medias: []*description.Media{medi}}

	var warnLogs []string
	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...any) {
			if l == logger.Warn {
				warnLogs = append(warnLogs, fmt.Sprintf(format, args...))
			}
		}),
	}

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)

	err := FromStream(desc, r, bw, fakeSRTConn{}, time.Second)
	require.NoError(t, err)

	cb := extractOnData(t, r, medi, medi.Formats[0])

	// IDR (NAL unit type 19, IDR_W_RADL) prefixed with H265 NAL header.
	idr := unit.PayloadH265{{0x26, 0x01, 0xaf, 0x08, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2}}

	require.NoError(t, cb(&unit.Unit{PTS: 90000, Payload: idr}))
	require.NoError(t, cb(&unit.Unit{PTS: 0, Payload: idr}))

	require.NotEmpty(t, warnLogs, "expected a warn log after the DTS error")
	require.True(t, strings.Contains(warnLogs[0], "H265 DTS extractor reset"),
		"unexpected warn log: %q", warnLogs[0])

	require.NoError(t, cb(&unit.Unit{PTS: 180000, Payload: idr}))
}
