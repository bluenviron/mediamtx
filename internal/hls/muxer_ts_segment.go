package hls

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/asticode/go-astits"
)

type muxerTSSegment struct {
	videoTrack gortsplib.Track
	writer     *muxerTSWriter

	name           string
	buf            bytes.Buffer
	startPTS       *time.Duration
	endPTS         time.Duration
	pcrSendCounter int
}

func newMuxerTSSegment(
	videoTrack gortsplib.Track,
	writer *muxerTSWriter,
) *muxerTSSegment {
	t := &muxerTSSegment{
		videoTrack: videoTrack,
		writer:     writer,
		name:       strconv.FormatInt(time.Now().Unix(), 10),
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
	if t.startPTS == nil {
		t.startPTS = &pts
	}

	var af *astits.PacketAdaptationField

	if idrPresent {
		if af == nil {
			af = &astits.PacketAdaptationField{}
		}
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
	return err
}

func (t *muxerTSSegment) writeAAC(
	startPCR time.Time,
	pts time.Duration,
	enc []byte) error {
	if t.startPTS == nil {
		t.startPTS = &pts
	}

	af := &astits.PacketAdaptationField{
		RandomAccessIndicator: true,
	}

	// if audio is the only track
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
	return err
}
