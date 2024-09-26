// Package hls contains HLS utilities.
package hls

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// ErrNoSupportedCodecs is returned by FromStream when there are no supported codecs.
var ErrNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently AV1, VP9, H265, H264, Opus, MPEG-4 Audio")

func setupVideoTrack(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	muxer *gohlslib.Muxer,
) format.Format {
	var videoFormatAV1 *format.AV1
	videoMedia := stream.Desc().FindFormat(&videoFormatAV1)

	if videoFormatAV1 != nil {
		stream.AddReader(writer, videoMedia, videoFormatAV1, func(u unit.Unit) error {
			tunit := u.(*unit.AV1)

			if tunit.TU == nil {
				return nil
			}

			err := muxer.WriteAV1(tunit.NTP, tunit.PTS, tunit.TU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		muxer.VideoTrack = &gohlslib.Track{
			Codec: &codecs.AV1{},
		}
		return videoFormatAV1
	}

	var videoFormatVP9 *format.VP9
	videoMedia = stream.Desc().FindFormat(&videoFormatVP9)

	if videoFormatVP9 != nil {
		stream.AddReader(writer, videoMedia, videoFormatVP9, func(u unit.Unit) error {
			tunit := u.(*unit.VP9)

			if tunit.Frame == nil {
				return nil
			}

			err := muxer.WriteVP9(tunit.NTP, tunit.PTS, tunit.Frame)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		muxer.VideoTrack = &gohlslib.Track{
			Codec: &codecs.VP9{},
		}
		return videoFormatVP9
	}

	var videoFormatH265 *format.H265
	videoMedia = stream.Desc().FindFormat(&videoFormatH265)

	if videoFormatH265 != nil {
		stream.AddReader(writer, videoMedia, videoFormatH265, func(u unit.Unit) error {
			tunit := u.(*unit.H265)

			if tunit.AU == nil {
				return nil
			}

			err := muxer.WriteH265(tunit.NTP, tunit.PTS, tunit.AU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		vps, sps, pps := videoFormatH265.SafeParams()

		muxer.VideoTrack = &gohlslib.Track{
			Codec: &codecs.H265{
				VPS: vps,
				SPS: sps,
				PPS: pps,
			},
		}
		return videoFormatH265
	}

	var videoFormatH264 *format.H264
	videoMedia = stream.Desc().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		stream.AddReader(writer, videoMedia, videoFormatH264, func(u unit.Unit) error {
			tunit := u.(*unit.H264)

			if tunit.AU == nil {
				return nil
			}

			err := muxer.WriteH264(tunit.NTP, tunit.PTS, tunit.AU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		sps, pps := videoFormatH264.SafeParams()

		muxer.VideoTrack = &gohlslib.Track{
			Codec: &codecs.H264{
				SPS: sps,
				PPS: pps,
			},
		}
		return videoFormatH264
	}

	return nil
}

func setupAudioTrack(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	muxer *gohlslib.Muxer,
) format.Format {
	var audioFormatOpus *format.Opus
	audioMedia := stream.Desc().FindFormat(&audioFormatOpus)

	if audioFormatOpus != nil {
		stream.AddReader(writer, audioMedia, audioFormatOpus, func(u unit.Unit) error {
			tunit := u.(*unit.Opus)

			err := muxer.WriteOpus(
				tunit.NTP,
				tunit.PTS,
				tunit.Packets)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		muxer.AudioTrack = &gohlslib.Track{
			Codec: &codecs.Opus{
				ChannelCount: audioFormatOpus.ChannelCount,
			},
		}
		return audioFormatOpus
	}

	var audioFormatMPEG4Audio *format.MPEG4Audio
	audioMedia = stream.Desc().FindFormat(&audioFormatMPEG4Audio)

	if audioFormatMPEG4Audio != nil {
		co := audioFormatMPEG4Audio.GetConfig()
		if co != nil {
			stream.AddReader(writer, audioMedia, audioFormatMPEG4Audio, func(u unit.Unit) error {
				tunit := u.(*unit.MPEG4Audio)

				if tunit.AUs == nil {
					return nil
				}

				err := muxer.WriteMPEG4Audio(
					tunit.NTP,
					tunit.PTS,
					tunit.AUs)
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

			muxer.AudioTrack = &gohlslib.Track{
				Codec: &codecs.MPEG4Audio{
					Config: *co,
				},
			}
			return audioFormatMPEG4Audio
		}
	}

	return nil
}

// FromStream maps a MediaMTX stream to a HLS muxer.
func FromStream(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	muxer *gohlslib.Muxer,
	l logger.Writer,
) error {
	videoFormat := setupVideoTrack(
		stream,
		writer,
		muxer,
	)

	audioFormat := setupAudioTrack(
		stream,
		writer,
		muxer,
	)

	if videoFormat == nil && audioFormat == nil {
		return ErrNoSupportedCodecs
	}

	n := 1
	for _, media := range stream.Desc().Medias {
		for _, forma := range media.Formats {
			if forma != videoFormat && forma != audioFormat {
				l.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	return nil
}
