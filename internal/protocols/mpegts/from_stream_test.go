package mpegts

import (
	"bufio"
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestFromStreamNoSupportedCodecs(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.VP8{}},
	}}}

	r := &stream.Reader{
		Parent: test.Logger(func(logger.Level, string, ...any) {
			t.Error("should not happen")
		}),
	}

	err := FromStream(desc, r, nil, nil, 0)
	require.Equal(t, errNoSupportedCodecs, err)
}

func TestFromStreamSkipUnsupportedTracks(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.H265{}},
		},
		{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{&format.VP8{}},
		},
	}}

	n := 0

	r := &stream.Reader{
		Parent: test.Logger(func(l logger.Level, format string, args ...any) {
			require.Equal(t, logger.Warn, l)
			if n == 0 {
				require.Equal(t, "skipping track 2 (VP8)", fmt.Sprintf(format, args...))
			}
			n++
		}),
	}

	err := FromStream(desc, r, nil, nil, 0)
	require.NoError(t, err)

	require.Equal(t, 1, n)
}

type lockedBuffer struct {
	mutex sync.Mutex
	buf   bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) bytes() []byte {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

type dummyConn struct {
	srt.Conn
}

func (dummyConn) SetWriteDeadline(time.Time) error { return nil }

func TestFromStreamLanguage(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:     description.MediaTypeVideo,
		Language: "it",
		Formats:  []format.Format{test.FormatH264},
	}}}

	strm := &stream.Stream{
		OrigDesc:          desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            test.NilLogger,
	}
	err := strm.Initialize()
	require.NoError(t, err)
	defer strm.Close()

	subStream := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = subStream.Initialize()
	require.NoError(t, err)

	buf := &lockedBuffer{}
	bw := bufio.NewWriter(buf)

	r := &stream.Reader{Parent: test.NilLogger}

	err = FromStream(desc, r, bw, dummyConn{}, time.Second)
	require.NoError(t, err)

	strm.AddReader(r)
	defer strm.RemoveReader(r)

	subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
		PTS: 90000,
		Payload: unit.PayloadH264{
			test.FormatH264.SPS,
			test.FormatH264.PPS,
			{5, 1},
		},
	})

	// wait for the PMT to be written
	var mr *mpegts.Reader
	require.Eventually(t, func() bool {
		mr = &mpegts.Reader{R: bytes.NewReader(buf.bytes())}
		return mr.Initialize() == nil
	}, 3*time.Second, 10*time.Millisecond)

	require.Len(t, mr.Tracks(), 1)
	require.Equal(t, "ita", mr.Tracks()[0].Language)
}
