package hls

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/abema/go-mp4"
	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
)

type fmp4SegmentEntry struct {
	pts  time.Duration
	avcc []byte
}

func mp4SegmentGenerate(
	sequenceNumber int,
	entries []fmp4SegmentEntry,
	relativeStartTime time.Duration,
) ([]byte, error) {
	/*
		moof
		- mfhd
		- traf
		  - tfhd
		  - tfdt
		  - trun
		  - trun
		  - ...
		mdat
	*/

	w := newMP4Writer()

	moofOffset, err := w.writeBoxStart(&mp4.Moof{}) // <moof>
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Mfhd{ // <mfhd/>
		SequenceNumber: uint32(sequenceNumber),
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBoxStart(&mp4.Traf{}) // <traf>
	if err != nil {
		return nil, err
	}

	// generate PTS by sorting DTS
	allPTS := make([]time.Duration, len(entries))
	for i, e := range entries {
		allPTS[i] = e.pts
	}
	sort.Slice(allPTS, func(i, j int) bool {
		return allPTS[i] < allPTS[j]
	})

	var sampleDuration time.Duration
	for i := 1; i < len(allPTS); i++ {
		sampleDuration += allPTS[i] - allPTS[i-1]
	}
	sampleDuration /= time.Duration(len(allPTS) - 1)

	_, err = w.writeBox(&mp4.Tfhd{ // <tfhd/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{2, 0, 56},
		},
		TrackID:               1,
		DefaultSampleDuration: uint32(sampleDuration * fmp4Timescale / time.Second),
	})
	if err != nil {
		return nil, err
	}

	_, err = w.writeBox(&mp4.Tfdt{ // <tfdt/>
		FullBox: mp4.FullBox{
			Version: 1,
		},
		// sum of decode durations of all earlier samples
		BaseMediaDecodeTimeV1: uint64(relativeStartTime * fmp4Timescale / time.Second),
	})
	if err != nil {
		return nil, err
	}

	trun := &mp4.Trun{ // <trun/>
		FullBox: mp4.FullBox{
			Version: 1,
			Flags:   [3]byte{0, 10, 5},
		},
		SampleCount: uint32(len(entries)),
	}

	for i, e := range entries {
		dts := allPTS[i]
		off := int32((e.pts-dts)*fmp4Timescale/time.Second) + fmp4PtsDtsOffset

		trun.Entries = append(trun.Entries, mp4.TrunEntry{
			SampleSize:                    uint32(len(e.avcc)),
			SampleCompositionTimeOffsetV1: off,
		})
	}

	trunOffset, err := w.writeBox(trun)
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </traf>
	if err != nil {
		return nil, err
	}

	err = w.writeBoxEnd() // </moof>
	if err != nil {
		return nil, err
	}

	mdat := &mp4.Mdat{} // <mdat/>

	size := 0
	for _, e := range entries {
		size += len(e.avcc)
	}

	data := make([]byte, size)
	pos := 0
	for _, e := range entries {
		pos += copy(data[pos:], e.avcc)
	}
	mdat.Data = data

	mdatOffset, err := w.writeBox(mdat)
	if err != nil {
		return nil, err
	}

	trun.DataOffset = int32(mdatOffset - moofOffset + 8)
	err = w.rewriteBox(trunOffset, trun)
	if err != nil {
		return nil, err
	}

	return w.bytes(), nil
}

type muxerVariantFMP4Segment struct {
	sequenceNumber int
	segmentMaxSize uint64
	videoTrack     *gortsplib.TrackH264
	audioTrack     *gortsplib.TrackAAC

	startTime    time.Time
	name         string
	startPTS     *time.Duration
	endPTS       time.Duration
	audioAUCount int
	entries      []fmp4SegmentEntry
	entriesSize  uint64
	output       []byte
}

func newMuxerVariantFMP4Segment(
	sequenceNumber int,
	startTime time.Time,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackAAC,
) *muxerVariantFMP4Segment {
	t := &muxerVariantFMP4Segment{
		sequenceNumber: sequenceNumber,
		segmentMaxSize: segmentMaxSize,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		startTime:      startTime,
		name:           strconv.FormatInt(startTime.Unix(), 10),
	}

	return t
}

func (t *muxerVariantFMP4Segment) duration() time.Duration {
	return t.endPTS - *t.startPTS
}

func (t *muxerVariantFMP4Segment) reader() io.Reader {
	return bytes.NewReader(t.output)
}

func (t *muxerVariantFMP4Segment) finalize(nextPTS *time.Duration, segmenterStartTime time.Time) error {
	if nextPTS != nil {
		t.endPTS = *nextPTS
	}

	var err error
	t.output, err = mp4SegmentGenerate(t.sequenceNumber, t.entries, t.startTime.Sub(segmenterStartTime))
	if err != nil {
		return err
	}

	t.entries = nil
	return nil
}

func (t *muxerVariantFMP4Segment) writeH264(
	pts time.Duration,
	idrPresent bool,
	nalus [][]byte,
) error {
	avcc, err := h264.AVCCEncode(nalus)
	if err != nil {
		return err
	}
	avccl := uint64(len(avcc))

	if (t.entriesSize + avccl) > t.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}

	t.entries = append(t.entries, fmp4SegmentEntry{
		pts:  pts - *t.startPTS,
		avcc: avcc,
	})
	t.entriesSize += avccl

	if pts > t.endPTS {
		t.endPTS = pts
	}

	return nil
}

func (t *muxerVariantFMP4Segment) writeAAC(
	pts time.Duration,
	aus [][]byte,
) error {
	if t.videoTrack == nil {
		t.audioAUCount += len(aus)
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}

	if pts > t.endPTS {
		t.endPTS = pts
	}

	return nil
}
