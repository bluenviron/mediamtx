package hls

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"

	"github.com/aler9/rtsp-simple-server/internal/hls/mpegts"
)

type muxerVariantMPEGTSSegment struct {
	segmentMaxSize uint64
	videoTrack     *gortsplib.TrackH264
	audioTrack     *gortsplib.TrackMPEG4Audio
	writer         *mpegts.Writer

	size         uint64
	startTime    time.Time
	name         string
	startDTS     *time.Duration
	endDTS       time.Duration
	audioAUCount int
	content      []byte
}

func newMuxerVariantMPEGTSSegment(
	id uint64,
	startTime time.Time,
	segmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
	writer *mpegts.Writer,
) *muxerVariantMPEGTSSegment {
	t := &muxerVariantMPEGTSSegment{
		segmentMaxSize: segmentMaxSize,
		videoTrack:     videoTrack,
		audioTrack:     audioTrack,
		writer:         writer,
		startTime:      startTime,
		name:           "seg" + strconv.FormatUint(id, 10),
	}

	return t
}

func (t *muxerVariantMPEGTSSegment) duration() time.Duration {
	return t.endDTS - *t.startDTS
}

func (t *muxerVariantMPEGTSSegment) reader() io.Reader {
	return bytes.NewReader(t.content)
}

func (t *muxerVariantMPEGTSSegment) finalize(endDTS time.Duration) {
	t.endDTS = endDTS
	t.content = t.writer.GenerateSegment()
}

func (t *muxerVariantMPEGTSSegment) writeH264(
	pcr time.Duration,
	dts time.Duration,
	pts time.Duration,
	idrPresent bool,
	nalus [][]byte,
) error {
	size := uint64(0)
	for _, nalu := range nalus {
		size += uint64(len(nalu))
	}
	if (t.size + size) > t.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	t.size += size

	err := t.writer.WriteH264(pcr, dts, pts, idrPresent, nalus)
	if err != nil {
		return err
	}

	if t.startDTS == nil {
		t.startDTS = &dts
	}
	t.endDTS = dts

	return nil
}

func (t *muxerVariantMPEGTSSegment) writeAAC(
	pcr time.Duration,
	pts time.Duration,
	au []byte,
) error {
	size := uint64(len(au))
	if (t.size + size) > t.segmentMaxSize {
		return fmt.Errorf("reached maximum segment size")
	}
	t.size += size

	err := t.writer.WriteAAC(pcr, pts, au)
	if err != nil {
		return err
	}

	if t.videoTrack == nil {
		t.audioAUCount++

		if t.startDTS == nil {
			t.startDTS = &pts
		}
		t.endDTS = pts
	}

	return nil
}
