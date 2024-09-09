package rtmp

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently H264, MPEG-4 Audio, MPEG-1/2 Audio")

func setupVideo(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	w **Writer,
	nconn net.Conn,
	writeTimeout time.Duration,
) format.Format {
	var videoFormatH264 *format.H264
	videoMedia := stream.Desc().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		var videoDTSExtractor *h264.DTSExtractor

		stream.AddReader(writer, videoMedia, videoFormatH264, func(u unit.Unit) error {
			tunit := u.(*unit.H264)

			if tunit.AU == nil {
				return nil
			}

			idrPresent := false
			nonIDRPresent := false

			for _, nalu := range tunit.AU {
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeIDR:
					idrPresent = true

				case h264.NALUTypeNonIDR:
					nonIDRPresent = true
				}
			}

			var dts time.Duration

			// wait until we receive an IDR
			if videoDTSExtractor == nil {
				if !idrPresent {
					return nil
				}

				videoDTSExtractor = h264.NewDTSExtractor()

				var err error
				dts, err = videoDTSExtractor.Extract(tunit.AU, tunit.PTS)
				if err != nil {
					return err
				}
			} else {
				if !idrPresent && !nonIDRPresent {
					return nil
				}

				var err error
				dts, err = videoDTSExtractor.Extract(tunit.AU, tunit.PTS)
				if err != nil {
					return err
				}
			}

			nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
			return (*w).WriteH264(tunit.PTS, dts, idrPresent, tunit.AU)
		})

		return videoFormatH264
	}

	return nil
}

func setupAudio(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	w **Writer,
	nconn net.Conn,
	writeTimeout time.Duration,
) format.Format {
	var audioFormatMPEG4Audio *format.MPEG4Audio
	audioMedia := stream.Desc().FindFormat(&audioFormatMPEG4Audio)

	if audioMedia != nil {
		stream.AddReader(writer, audioMedia, audioFormatMPEG4Audio, func(u unit.Unit) error {
			tunit := u.(*unit.MPEG4Audio)

			if tunit.AUs == nil {
				return nil
			}

			for i, au := range tunit.AUs {
				nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
				err := (*w).WriteMPEG4Audio(
					tunit.PTS+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
						time.Second/time.Duration(audioFormatMPEG4Audio.ClockRate()),
					au,
				)
				if err != nil {
					return err
				}
			}

			return nil
		})

		return audioFormatMPEG4Audio
	}

	var audioFormatMPEG1 *format.MPEG1Audio
	audioMedia = stream.Desc().FindFormat(&audioFormatMPEG1)

	if audioMedia != nil {
		stream.AddReader(writer, audioMedia, audioFormatMPEG1, func(u unit.Unit) error {
			tunit := u.(*unit.MPEG1Audio)

			pts := tunit.PTS

			for _, frame := range tunit.Frames {
				var h mpeg1audio.FrameHeader
				err := h.Unmarshal(frame)
				if err != nil {
					return err
				}

				if !(!h.MPEG2 && h.Layer == 3) {
					return fmt.Errorf("RTMP only supports MPEG-1 layer 3 audio")
				}

				nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
				err = (*w).WriteMPEG1Audio(pts, &h, frame)
				if err != nil {
					return err
				}

				pts += time.Duration(h.SampleCount()) *
					time.Second / time.Duration(h.SampleRate)
			}

			return nil
		})

		return audioFormatMPEG1
	}

	return nil
}

// FromStream maps a MediaMTX stream to a RTMP stream.
func FromStream(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	conn *Conn,
	nconn net.Conn,
	writeTimeout time.Duration,
) error {
	var w *Writer

	videoFormat := setupVideo(
		stream,
		writer,
		&w,
		nconn,
		writeTimeout,
	)

	audioFormat := setupAudio(
		stream,
		writer,
		&w,
		nconn,
		writeTimeout,
	)

	if videoFormat == nil && audioFormat == nil {
		return errNoSupportedCodecs
	}

	var err error
	w, err = NewWriter(conn, videoFormat, audioFormat)
	if err != nil {
		return err
	}

	return nil
}
