// Package mpegts contains a MPEG-TS reader and writer.
package mpegts

import (
	"bytes"
	"context"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"
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
	videoTrack *gortsplib.TrackH264
	audioTrack *gortsplib.TrackMPEG4Audio

	buf        *bytes.Buffer
	inner      *astits.Muxer
	pcrCounter int
}

// NewWriter allocates a Writer.
func NewWriter(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
) *Writer {
	w := &Writer{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		buf:        bytes.NewBuffer(nil),
	}

	w.inner = astits.NewMuxer(
		context.Background(),
		writerFunc(func(p []byte) (int, error) {
			return w.buf.Write(p)
		}))

	if videoTrack != nil {
		w.inner.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 256,
			StreamType:    astits.StreamTypeH264Video,
		})
	}

	if audioTrack != nil {
		w.inner.AddElementaryStream(astits.PMTElementaryStream{
			ElementaryPID: 257,
			StreamType:    astits.StreamTypeAACAudio,
		})
	}

	if videoTrack != nil {
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

// WriteH264 writes a group of H264 NALUs.
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
			Type:         w.audioTrack.Config.Type,
			SampleRate:   w.audioTrack.Config.SampleRate,
			ChannelCount: w.audioTrack.Config.ChannelCount,
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

	if w.videoTrack == nil {
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
