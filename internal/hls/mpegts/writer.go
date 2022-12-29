// Package mpegts contains a MPEG-TS reader and writer.
package mpegts

import (
	"bytes"
	"context"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/asticode/go-astits"
)

const (
	pcrOffset = 400 * time.Millisecond // 2 samples @ 5fps
)

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

// Writer is a MPEG-TS writer.
type Writer struct {
	videoFormat *format.H264
	audioFormat *format.MPEG4Audio

	buf        *bytes.Buffer
	inner      *astits.Muxer
	pcrCounter int
}

// NewWriter allocates a Writer.
func NewWriter(
	videoFormat *format.H264,
	audioFormat *format.MPEG4Audio,
) *Writer {
	w := &Writer{
		videoFormat: videoFormat,
		audioFormat: audioFormat,
		buf:         bytes.NewBuffer(nil),
	}

	w.inner = astits.NewMuxer(
		context.Background(),
		writerFunc(func(p []byte) (int, error) {
			return w.buf.Write(p)
		}))

	if videoFormat != nil {
		w.inner.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    astits.StreamTypeH264Video,
		})
	}

	if audioFormat != nil {
		w.inner.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    astits.StreamTypeAACAudio,
		})
	}

	if videoFormat != nil {
		w.inner.SetPCRPID(256)
	} else {
		w.inner.SetPCRPID(257)
	}

	// WriteTable() is not necessary
	// since it's called automatically when WriteData() is called with
	// * PID == PCRPID
	// * AdaptationField != nil
	// * RandomAccessIndicator = true

	return w
}

// GenerateSegment generates a MPEG-TS segment.
func (w *Writer) GenerateSegment() []byte {
	w.pcrCounter = 0
	ret := w.buf.Bytes()
	w.buf = bytes.NewBuffer(nil)
	return ret
}

// WriteH264 writes a H264 access unit.
func (w *Writer) WriteH264(
	pcr time.Duration,
	dts time.Duration,
	pts time.Duration,
	idrPresent bool,
	nalus [][]byte,
) error {
	// prepend an AUD. This is required by video.js and iOS
	nalus = append([][]byte{{byte(h264.NALUTypeAccessUnitDelimiter), 240}}, nalus...)

	enc, err := h264.AnnexBMarshal(nalus)
	if err != nil {
		return err
	}

	var af *astits.PacketAdaptationField

	if idrPresent {
		af = &astits.PacketAdaptationField{}
		af.RandomAccessIndicator = true
	}

	// send PCR once in a while
	if w.pcrCounter == 0 {
		if af == nil {
			af = &astits.PacketAdaptationField{}
		}
		af.HasPCR = true
		af.PCR = &astits.ClockReference{Base: int64(pcr.Seconds() * 90000)}
		w.pcrCounter = 3
	}
	w.pcrCounter--

	oh := &astits.PESOptionalHeader{
		MarkerBits: 2,
	}

	if dts == pts {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorOnlyPTS
		oh.PTS = &astits.ClockReference{Base: int64((pts + pcrOffset).Seconds() * 90000)}
	} else {
		oh.PTSDTSIndicator = astits.PTSDTSIndicatorBothPresent
		oh.DTS = &astits.ClockReference{Base: int64((dts + pcrOffset).Seconds() * 90000)}
		oh.PTS = &astits.ClockReference{Base: int64((pts + pcrOffset).Seconds() * 90000)}
	}

	_, err = w.inner.WriteData(&astits.MuxerData{
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

// WriteAAC writes an AAC AU.
func (w *Writer) WriteAAC(
	pcr time.Duration,
	pts time.Duration,
	au []byte,
) error {
	pkts := mpeg4audio.ADTSPackets{
		{
			Type:         w.audioFormat.Config.Type,
			SampleRate:   w.audioFormat.Config.SampleRate,
			ChannelCount: w.audioFormat.Config.ChannelCount,
			AU:           au,
		},
	}

	enc, err := pkts.Marshal()
	if err != nil {
		return err
	}

	af := &astits.PacketAdaptationField{
		RandomAccessIndicator: true,
	}

	if w.videoFormat == nil {
		// send PCR once in a while
		if w.pcrCounter == 0 {
			af.HasPCR = true
			af.PCR = &astits.ClockReference{Base: int64(pcr.Seconds() * 90000)}
			w.pcrCounter = 3
		}
		w.pcrCounter--
	}

	_, err = w.inner.WriteData(&astits.MuxerData{
		PID:             257,
		AdaptationField: af,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
					PTS:             &astits.ClockReference{Base: int64((pts + pcrOffset).Seconds() * 90000)},
				},
				PacketLength: uint16(len(enc) + 8),
				StreamID:     192, // audio
			},
			Data: enc,
		},
	})
	return err
}
