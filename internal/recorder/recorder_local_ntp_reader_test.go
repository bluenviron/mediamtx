package recorder

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestRecorderReaderClockScope(t *testing.T) {
	for _, local := range []bool{true, false} {
		strm := &stream.Stream{
			OrigDesc: &description.Session{Medias: []*description.Media{{
				Type: description.MediaTypeAudio,
				Formats: []rtspformat.Format{&rtspformat.LPCM{
					PayloadTyp: 96, BitDepth: 16, SampleRate: 8000, ChannelCount: 1,
				}},
			}}},
			ReplaceNTP: local, WriteQueueSize: 512, RTPMaxPayloadSize: 1450, Parent: test.NilLogger,
		}
		require.NoError(t, strm.Initialize())
		reader := &stream.Reader{Parent: test.NilLogger}
		stamp := time.Unix(1700000000, 0)
		f := &formatFMP4{
			ri:  &recorderInstance{stream: strm, reader: reader, parent: test.NilLogger},
			now: func() time.Time { return stamp },
		}
		require.True(t, f.initialize())
		if local {
			require.NotNil(t, reader.TimeNow)
			require.Equal(t, stamp, reader.TimeNow())
		} else {
			require.Nil(t, reader.TimeNow)
		}
		strm.Close()
	}
}
