package test

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"
)

// WriteRecordingSegment writes a recording segment in fMP4 format.
func WriteRecordingSegment(t *testing.T, fpath string, duration time.Duration, cached bool) {
	t.Helper()

	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{{
			ID:        1,
			TimeScale: 90000,
			Codec: &mcodecs.H264{
				SPS: FormatH264.SPS,
				PPS: FormatH264.PPS,
			},
		}},
	}

	var buf seekablebuffer.Buffer
	err := init.Marshal(&buf)
	require.NoError(t, err)

	if cached {
		boxes, err2 := amp4.ExtractBoxWithPayload(bytes.NewReader(buf.Bytes()), nil,
			amp4.BoxPath{amp4.BoxTypeMoov(), amp4.BoxTypeMvhd()})
		require.NoError(t, err2)
		require.Len(t, boxes, 1)

		mvhd := boxes[0].Payload.(*amp4.Mvhd)
		mvhd.DurationV0 = uint32(duration * time.Duration(mvhd.Timescale) / time.Second)
		_, err = buf.Seek(int64(boxes[0].Info.Offset+boxes[0].Info.HeaderSize), io.SeekStart)
		require.NoError(t, err)
		_, err = amp4.Marshal(&buf, mvhd, boxes[0].Info.Context)
		require.NoError(t, err)
	} else {
		half := uint32(duration * 90000 / time.Second / 2)
		parts := fmp4.Parts{
			{
				Tracks: []*fmp4.PartTrack{{
					ID: 1,
					Samples: []*fmp4.Sample{{
						Duration: half,
						Payload:  []byte{1, 2},
					}},
				}},
			},
			{
				SequenceNumber: 1,
				Tracks: []*fmp4.PartTrack{{
					ID:       1,
					BaseTime: uint64(half),
					Samples: []*fmp4.Sample{{
						Duration: half,
						Payload:  []byte{3, 4},
					}},
				}},
			},
		}
		err = parts.Marshal(&buf)
		require.NoError(t, err)
	}

	err = os.WriteFile(fpath, buf.Bytes(), 0o644)
	require.NoError(t, err)
}
