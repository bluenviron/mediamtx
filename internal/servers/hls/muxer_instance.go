package hls

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/gin-gonic/gin"
)

var errNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently H265, H264, Opus, MPEG-4 Audio")

type muxerInstance struct {
	variant         conf.HLSVariant
	segmentCount    int
	segmentDuration conf.StringDuration
	partDuration    conf.StringDuration
	segmentMaxSize  conf.StringSize
	directory       string
	writeQueueSize  int
	pathName        string
	stream          *stream.Stream
	bytesSent       *uint64
	parent          logger.Writer

	writer *asyncwriter.Writer
	hmuxer *gohlslib.Muxer
}

func (mi *muxerInstance) initialize() error {
	mi.writer = asyncwriter.New(mi.writeQueueSize, mi)

	videoTrack := mi.createVideoTrack()
	audioTrack := mi.createAudioTrack()

	if videoTrack == nil && audioTrack == nil {
		mi.stream.RemoveReader(mi.writer)
		return errNoSupportedCodecs
	}

	var muxerDirectory string
	if mi.directory != "" {
		muxerDirectory = filepath.Join(mi.directory, mi.pathName)
		os.MkdirAll(muxerDirectory, 0o755)
	}

	mi.hmuxer = &gohlslib.Muxer{
		Variant:         gohlslib.MuxerVariant(mi.variant),
		SegmentCount:    mi.segmentCount,
		SegmentDuration: time.Duration(mi.segmentDuration),
		PartDuration:    time.Duration(mi.partDuration),
		SegmentMaxSize:  uint64(mi.segmentMaxSize),
		VideoTrack:      videoTrack,
		AudioTrack:      audioTrack,
		Directory:       muxerDirectory,
	}

	err := mi.hmuxer.Start()
	if err != nil {
		mi.stream.RemoveReader(mi.writer)
		return err
	}

	mi.Log(logger.Info, "is converting into HLS, %s",
		defs.FormatsInfo(mi.stream.FormatsForReader(mi.writer)))

	mi.writer.Start()

	return nil
}

// Log implements logger.Writer.
func (mi *muxerInstance) Log(level logger.Level, format string, args ...interface{}) {
	mi.parent.Log(level, format, args...)
}

func (mi *muxerInstance) close() {
	mi.writer.Stop()
	mi.hmuxer.Close()
	mi.stream.RemoveReader(mi.writer)
	if mi.hmuxer.Directory != "" {
		os.Remove(mi.hmuxer.Directory)
	}
}

func (mi *muxerInstance) createVideoTrack() *gohlslib.Track {
	var videoFormatAV1 *format.AV1
	videoMedia := mi.stream.Desc().FindFormat(&videoFormatAV1)

	if videoFormatAV1 != nil {
		mi.stream.AddReader(mi.writer, videoMedia, videoFormatAV1, func(u unit.Unit) error {
			tunit := u.(*unit.AV1)

			if tunit.TU == nil {
				return nil
			}

			err := mi.hmuxer.WriteAV1(tunit.NTP, tunit.PTS, tunit.TU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return &gohlslib.Track{
			Codec: &codecs.AV1{},
		}
	}

	var videoFormatVP9 *format.VP9
	videoMedia = mi.stream.Desc().FindFormat(&videoFormatVP9)

	if videoFormatVP9 != nil {
		mi.stream.AddReader(mi.writer, videoMedia, videoFormatVP9, func(u unit.Unit) error {
			tunit := u.(*unit.VP9)

			if tunit.Frame == nil {
				return nil
			}

			err := mi.hmuxer.WriteVP9(tunit.NTP, tunit.PTS, tunit.Frame)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return &gohlslib.Track{
			Codec: &codecs.VP9{},
		}
	}

	var videoFormatH265 *format.H265
	videoMedia = mi.stream.Desc().FindFormat(&videoFormatH265)

	if videoFormatH265 != nil {
		mi.stream.AddReader(mi.writer, videoMedia, videoFormatH265, func(u unit.Unit) error {
			tunit := u.(*unit.H265)

			if tunit.AU == nil {
				return nil
			}

			err := mi.hmuxer.WriteH265(tunit.NTP, tunit.PTS, tunit.AU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		vps, sps, pps := videoFormatH265.SafeParams()

		return &gohlslib.Track{
			Codec: &codecs.H265{
				VPS: vps,
				SPS: sps,
				PPS: pps,
			},
		}
	}

	var videoFormatH264 *format.H264
	videoMedia = mi.stream.Desc().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		mi.stream.AddReader(mi.writer, videoMedia, videoFormatH264, func(u unit.Unit) error {
			tunit := u.(*unit.H264)

			if tunit.AU == nil {
				return nil
			}

			err := mi.hmuxer.WriteH264(tunit.NTP, tunit.PTS, tunit.AU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		sps, pps := videoFormatH264.SafeParams()

		return &gohlslib.Track{
			Codec: &codecs.H264{
				SPS: sps,
				PPS: pps,
			},
		}
	}

	return nil
}

func (mi *muxerInstance) createAudioTrack() *gohlslib.Track {
	var audioFormatOpus *format.Opus
	audioMedia := mi.stream.Desc().FindFormat(&audioFormatOpus)

	if audioMedia != nil {
		mi.stream.AddReader(mi.writer, audioMedia, audioFormatOpus, func(u unit.Unit) error {
			tunit := u.(*unit.Opus)

			err := mi.hmuxer.WriteOpus(
				tunit.NTP,
				tunit.PTS,
				tunit.Packets)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return &gohlslib.Track{
			Codec: &codecs.Opus{
				ChannelCount: audioFormatOpus.ChannelCount,
			},
		}
	}

	var audioFormatMPEG4Audio *format.MPEG4Audio
	audioMedia = mi.stream.Desc().FindFormat(&audioFormatMPEG4Audio)

	if audioMedia != nil {
		mi.stream.AddReader(mi.writer, audioMedia, audioFormatMPEG4Audio, func(u unit.Unit) error {
			tunit := u.(*unit.MPEG4Audio)

			if tunit.AUs == nil {
				return nil
			}

			err := mi.hmuxer.WriteMPEG4Audio(
				tunit.NTP,
				tunit.PTS,
				tunit.AUs)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return &gohlslib.Track{
			Codec: &codecs.MPEG4Audio{
				Config: *audioFormatMPEG4Audio.GetConfig(),
			},
		}
	}

	return nil
}

func (mi *muxerInstance) errorChan() chan error {
	return mi.writer.Error()
}

func (mi *muxerInstance) handleRequest(ctx *gin.Context) {
	w := &responseWriterWithCounter{
		ResponseWriter: ctx.Writer,
		bytesSent:      mi.bytesSent,
	}

	mi.hmuxer.Handle(w, ctx.Request)
}
