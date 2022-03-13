package hls

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/asticode/go-astits"
)

type muxerTSSegment struct {
	hlsSegmentMaxSize uint64
	videoTrack        *gortsplib.TrackH264
	writer            *muxerTSWriter

	startTime      time.Time
	name           string
	buf            bytes.Buffer
	startPTS       *time.Duration
	endPTS         time.Duration
	pcrSendCounter int
	audioAUCount   int
}

func newMuxerTSSegment(
	hlsSegmentMaxSize uint64,
	videoTrack *gortsplib.TrackH264,
	writer *muxerTSWriter,
) *muxerTSSegment {
	now := time.Now()

	t := &muxerTSSegment{
		hlsSegmentMaxSize: hlsSegmentMaxSize,
		videoTrack:        videoTrack,
		writer:            writer,
		startTime:         now,
		name:              strconv.FormatInt(now.Unix(), 10),
	}

	// WriteTable() is called automatically when WriteData() is called with
	// - PID == PCRPID
	// - AdaptationField != nil
	// - RandomAccessIndicator = true

	writer.currentSegment = t

	return t
}

func (t *muxerTSSegment) duration() time.Duration {
	return t.endPTS - *t.startPTS
}

func (t *muxerTSSegment) write(p []byte) (int, error) {
	if uint64(len(p)+t.buf.Len()) > t.hlsSegmentMaxSize {
		return 0, fmt.Errorf("reached maximum segment size")
	}

	return t.buf.Write(p)
}

func (t *muxerTSSegment) reader() io.Reader {
	return bytes.NewReader(t.buf.Bytes())
}

func (t *muxerTSSegment) writeH264(
	startPCR time.Time,
	dts time.Duration,
	pts time.Duration,
	idrPresent bool,
	enc []byte) error {
	var af *astits.PacketAdaptationField

	if idrPresent {
		af = &astits.PacketAdaptationField{}
		af.RandomAccessIndicator = true
	}

	// send PCR once in a while
	if t.pcrSendCounter == 0 {
		if af == nil {
			af = &astits.PacketAdaptationField{}
		}
		af.HasPCR = true
		af.PCR = &astits.ClockReference{Base: int64(time.Since(startPCR).Seconds() * 90000)}
		t.pcrSendCounter = 3
	}
	t.pcrSendCounter--

	oh := &astits.PESOptionalHeader{
		MarkerBits: 2,
	}

	if dts == pts {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorOnlyPTS
		oh.PTS = &astits.ClockReference{Base: int64(pts.Seconds() * 90000)}
	} else {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorBothPresent
		oh.DTS = &astits.ClockReference{Base: int64(dts.Seconds() * 90000)}
		oh.PTS = &astits.ClockReference{Base: int64(pts.Seconds() * 90000)}
	}

	_, err := t.writer.WriteData(&astits.MuxerData{
		PID:             256,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: oh,
				StreamID:       224, // video
			},
			Data: enc,
		},
	})
	if err != nil {
		return err
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}

	if pts > t.endPTS {
		t.endPTS = pts
	}

	return nil
}

func (t *muxerTSSegment) writeAAC(
	startPCR time.Time,
	pts time.Duration,
	enc []byte,
	ausLen int) error {
	af := &astits.PacketAdaptationField{
		RandomAccessIndicator: true,
	}

	if t.videoTrack == nil {
		// send PCR once in a while
		if t.pcrSendCounter == 0 {
			af.HasPCR = true
			af.PCR = &astits.ClockReference{Base: int64(time.Since(startPCR).Seconds() * 90000)}
			t.pcrSendCounter = 3
		}
	}

	_, err := t.writer.WriteData(&astits.MuxerData{
		PID:             257,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64(pts.Seconds() * 90000)},
				},
				PacketLength: uint16(len(enc) + 8),
				StreamID:     192, // audio
			},
			Data: enc,
		},
	})
	if err != nil {
		return err
	}

	if t.videoTrack == nil {
		t.audioAUCount += ausLen
	}

	if t.startPTS == nil {
		t.startPTS = &pts
	}

	if pts > t.endPTS {
		t.endPTS = pts
	}

	return nil
}
