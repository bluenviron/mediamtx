package recorder

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type boxSpan struct {
	typ  string
	pos  int64
	size uint32
}

func listBoxes(t *testing.T, byts []byte) []boxSpan {
	var spans []boxSpan
	for pos := int64(0); pos < int64(len(byts)); {
		require.LessOrEqual(t, pos+8, int64(len(byts)))
		size := binary.BigEndian.Uint32(byts[pos:])
		require.GreaterOrEqual(t, size, uint32(8))
		spans = append(spans, boxSpan{
			typ:  string(byts[pos+4 : pos+8]),
			pos:  pos,
			size: size,
		})
		pos += int64(size)
	}
	return spans
}

func TestRecorderFMP4SeekIndex(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{{
		Type:    description.MediaTypeVideo,
		Formats: []rtspformat.Format{test.FormatH264},
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

	dir := t.TempDir()

	w := &Recorder{
		PathFormat:      filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     50 * 1024 * 1024,
		SegmentDuration: 10 * time.Second,
		PathName:        "mypath",
		Stream:          strm,
		Parent:          test.NilLogger,
	}
	w.Initialize()

	// 1 second of IDR samples, 10 per second -> 10 parts
	for i := range 11 {
		subStream.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS: int64(i) * 9000,
			NTP: time.Date(2008, 5, 20, 22, 15, 25, 0, time.UTC).Add(time.Duration(i) * 100 * time.Millisecond),
			Payload: unit.PayloadH264{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5}, // IDR
			},
		})
	}

	time.Sleep(100 * time.Millisecond)

	w.Close()

	byts, err := os.ReadFile(filepath.Join(dir, "mypath", "2008-05-20_22-15-25-000000.mp4"))
	require.NoError(t, err)

	spans := listBoxes(t, byts)

	// expected layout: ftyp, moov, free, sidx, then (moof, mdat) pairs
	require.Equal(t, "ftyp", spans[0].typ)
	require.Equal(t, "moov", spans[1].typ)
	require.Equal(t, "free", spans[2].typ)
	require.Equal(t, "sidx", spans[3].typ)

	var moofSizes []uint32
	for i := 4; i < len(spans); i += 2 {
		require.Equal(t, "moof", spans[i].typ)
		require.Equal(t, "mdat", spans[i+1].typ)
		moofSizes = append(moofSizes, spans[i].size+spans[i+1].size)
	}
	require.Equal(t, 10, len(moofSizes))

	var sidx amp4.Sidx
	_, err = amp4.Unmarshal(
		bytes.NewReader(byts[spans[3].pos+8:spans[3].pos+int64(spans[3].size)]),
		uint64(spans[3].size-8), &sidx, amp4.Context{})
	require.NoError(t, err)

	require.Equal(t, uint8(1), sidx.Version)
	require.Equal(t, uint32(1), sidx.ReferenceID)
	require.Equal(t, uint32(90000), sidx.Timescale)
	require.Equal(t, uint64(0), sidx.EarliestPresentationTimeV1)

	// the sidx must end exactly where the first moof starts
	require.Equal(t, uint64(0), sidx.FirstOffsetV1)
	require.Equal(t, spans[4].pos, spans[3].pos+int64(spans[3].size))

	// reserved = segmentDuration / partDuration + 2 = 102 entries; 10 used
	require.Equal(t, uint16(10), sidx.ReferenceCount)
	require.Equal(t, uint32((102-10)*seekIndexEntrySize), spans[2].size)

	var totalDuration uint64
	for i, ref := range sidx.References {
		require.Equal(t, moofSizes[i], ref.ReferencedSize)
		require.True(t, ref.StartsWithSAP)
		require.Equal(t, uint32(1), ref.SAPType)
		totalDuration += uint64(ref.SubsegmentDuration)
	}

	// 10 parts x 100ms at timescale 90000
	require.Equal(t, uint64(90000), totalDuration)

	// track durations must be written too
	moov := byts[spans[1].pos : spans[1].pos+int64(spans[1].size)]
	moovSpans := listBoxes(t, moov[8:])

	require.Equal(t, "mvhd", moovSpans[0].typ)
	var mvhd amp4.Mvhd
	_, err = amp4.Unmarshal(
		bytes.NewReader(moov[8+moovSpans[0].pos+8:]),
		uint64(moovSpans[0].size-8), &mvhd, amp4.Context{})
	require.NoError(t, err)
	require.Equal(t, uint32(1000), mvhd.Timescale)
	require.Equal(t, uint32(1000), mvhd.DurationV0)

	require.Equal(t, "trak", moovSpans[1].typ)
	trak := moov[8+moovSpans[1].pos : 8+moovSpans[1].pos+int64(moovSpans[1].size)]
	trakSpans := listBoxes(t, trak[8:])

	require.Equal(t, "tkhd", trakSpans[0].typ)
	var tkhd amp4.Tkhd
	_, err = amp4.Unmarshal(
		bytes.NewReader(trak[8+trakSpans[0].pos+8:]),
		uint64(trakSpans[0].size-8), &tkhd, amp4.Context{})
	require.NoError(t, err)
	require.Equal(t, uint32(1000), tkhd.DurationV0)

	require.Equal(t, "mdia", trakSpans[1].typ)
	mdia := trak[8+trakSpans[1].pos : 8+trakSpans[1].pos+int64(trakSpans[1].size)]
	mdiaSpans := listBoxes(t, mdia[8:])

	require.Equal(t, "mdhd", mdiaSpans[0].typ)
	var mdhd amp4.Mdhd
	_, err = amp4.Unmarshal(
		bytes.NewReader(mdia[8+mdiaSpans[0].pos+8:]),
		uint64(mdiaSpans[0].size-8), &mdhd, amp4.Context{})
	require.NoError(t, err)
	require.Equal(t, uint32(90000), mdhd.Timescale)
	require.Equal(t, uint32(90000), mdhd.DurationV0)
}

func writeTestSeekIndex(t *testing.T, reserved int, entries []seekIndexEntry) amp4.Sidx {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	err = writeSeekIndexPlaceholder(f, 1, reserved)
	require.NoError(t, err)

	tracks := []*formatFMP4Track{{
		initTrack: &fmp4.InitTrack{
			ID:        7,
			TimeScale: 90000,
			Codec: &mcodecs.H264{
				SPS: test.FormatH264.SPS,
				PPS: test.FormatH264.PPS,
			},
		},
	}}

	si := seekIndex{reserved: reserved, entries: entries}
	err = si.write(f, 5*time.Second, tracks)
	require.NoError(t, err)

	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	byts, err := io.ReadAll(f)
	require.NoError(t, err)

	spans := listBoxes(t, byts)
	last := spans[len(spans)-1]
	require.Equal(t, "sidx", last.typ)

	var sidx amp4.Sidx
	_, err = amp4.Unmarshal(
		bytes.NewReader(byts[last.pos+8:last.pos+int64(last.size)]),
		uint64(last.size-8), &sidx, amp4.Context{})
	require.NoError(t, err)

	require.Equal(t, uint32(7), sidx.ReferenceID)
	require.Equal(t, uint64(0), sidx.FirstOffsetV1)
	return sidx
}

func syncEntry(size uint64, start time.Duration, sap bool) seekIndexEntry {
	return seekIndexEntry{
		size: size,
		tracks: []seekIndexEntryTrack{{
			present:  true,
			baseTime: uint64(start * 90000 / time.Second),
			sap:      sap,
		}},
	}
}

func TestWriteSeekIndexMerge(t *testing.T) {
	for _, ca := range []struct {
		name     string
		reserved int
		saps     []bool
	}{
		{
			// entries whose reference track doesn't start with a sync sample
			// must be merged with the previous one.
			name:     "sync merge",
			reserved: 10,
			saps:     []bool{true, false, false, true, false},
		},
		{
			// with more sync samples than reserved entries, entries must be
			// coalesced to fit the reserved space.
			name:     "coalesce",
			reserved: 2,
			saps:     []bool{true, true, true, true, true},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var entries []seekIndexEntry
			for i, sap := range ca.saps {
				entries = append(entries, syncEntry(uint64(100*(i+1)), time.Duration(i)*time.Second, sap))
			}

			sidx := writeTestSeekIndex(t, ca.reserved, entries)

			// in both cases the expected groups are [100+200+300, 400+500]
			require.Equal(t, uint16(2), sidx.ReferenceCount)

			require.Equal(t, uint32(600), sidx.References[0].ReferencedSize)
			require.True(t, sidx.References[0].StartsWithSAP)
			require.Equal(t, uint32(3*90000), sidx.References[0].SubsegmentDuration)

			require.Equal(t, uint32(900), sidx.References[1].ReferencedSize)
			require.True(t, sidx.References[1].StartsWithSAP)
			require.Equal(t, uint32(2*90000), sidx.References[1].SubsegmentDuration)
		})
	}
}
