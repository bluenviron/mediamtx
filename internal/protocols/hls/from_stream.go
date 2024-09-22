// Package hls contains HLS utilities.
package hls

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
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
	setuppedFormats map[format.Format]struct{},
) {
	var videoFormatAV1 *format.AV1
	videoMedia := stream.Desc().FindFormat(&videoFormatAV1)

	if videoFormatAV1 != nil {
		track := &gohlslib.Track{
			Codec: &codecs.AV1{},
		}
		muxer.Tracks = append(muxer.Tracks, track)
		setuppedFormats[videoFormatAV1] = struct{}{}

		stream.AddReader(writer, videoMedia, videoFormatAV1, func(u unit.Unit) error {
			tunit := u.(*unit.AV1)

			if tunit.TU == nil {
				return nil
			}

			err := muxer.WriteAV1(track, tunit.NTP, tunit.PTS, tunit.TU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return
	}

	var videoFormatVP9 *format.VP9
	videoMedia = stream.Desc().FindFormat(&videoFormatVP9)

	if videoFormatVP9 != nil {
		track := &gohlslib.Track{
			Codec: &codecs.VP9{},
		}
		muxer.Tracks = append(muxer.Tracks, track)
		setuppedFormats[videoFormatVP9] = struct{}{}

		stream.AddReader(writer, videoMedia, videoFormatVP9, func(u unit.Unit) error {
			tunit := u.(*unit.VP9)

			if tunit.Frame == nil {
				return nil
			}

			err := muxer.WriteVP9(track, tunit.NTP, tunit.PTS, tunit.Frame)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return
	}

	var videoFormatH265 *format.H265
	videoMedia = stream.Desc().FindFormat(&videoFormatH265)

	if videoFormatH265 != nil {
		vps, sps, pps := videoFormatH265.SafeParams()
		track := &gohlslib.Track{
			Codec: &codecs.H265{
				VPS: vps,
				SPS: sps,
				PPS: pps,
			},
		}
		muxer.Tracks = append(muxer.Tracks, track)
		setuppedFormats[videoFormatH265] = struct{}{}

		stream.AddReader(writer, videoMedia, videoFormatH265, func(u unit.Unit) error {
			tunit := u.(*unit.H265)

			if tunit.AU == nil {
				return nil
			}

			err := muxer.WriteH265(track, tunit.NTP, tunit.PTS, tunit.AU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return
	}

	var videoFormatH264 *format.H264
	videoMedia = stream.Desc().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		sps, pps := videoFormatH264.SafeParams()
		track := &gohlslib.Track{
			Codec: &codecs.H264{
				SPS: sps,
				PPS: pps,
			},
		}
		muxer.Tracks = append(muxer.Tracks, track)
		setuppedFormats[videoFormatH264] = struct{}{}

		stream.AddReader(writer, videoMedia, videoFormatH264, func(u unit.Unit) error {
			tunit := u.(*unit.H264)

			if tunit.AU == nil {
				return nil
			}

			err := muxer.WriteH264(track, tunit.NTP, tunit.PTS, tunit.AU)
			if err != nil {
				return fmt.Errorf("muxer error: %w", err)
			}

			return nil
		})

		return
	}
}

func setupAudioTracks(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	muxer *gohlslib.Muxer,
	setuppedFormats map[format.Format]struct{},
) {
	for _, media := range stream.Desc().Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.Opus:
				track := &gohlslib.Track{
					Codec: &codecs.Opus{
						ChannelCount: forma.ChannelCount,
					},
				}
				muxer.Tracks = append(muxer.Tracks, track)
				setuppedFormats[forma] = struct{}{}

				stream.AddReader(writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.Opus)

					err := muxer.WriteOpus(
						track,
						tunit.NTP,
						tunit.PTS,
						tunit.Packets)
					if err != nil {
						return fmt.Errorf("muxer error: %w", err)
					}

					return nil
				})

			case *format.MPEG4Audio:
				co := forma.GetConfig()
				if co != nil {
					track := &gohlslib.Track{
						Codec: &codecs.MPEG4Audio{
							Config: *co,
						},
					}
					muxer.Tracks = append(muxer.Tracks, track)
					setuppedFormats[forma] = struct{}{}

					stream.AddReader(writer, media, forma, func(u unit.Unit) error {
						tunit := u.(*unit.MPEG4Audio)

						if tunit.AUs == nil {
							return nil
						}

						err := muxer.WriteMPEG4Audio(
							track,
							tunit.NTP,
							tunit.PTS,
							tunit.AUs)
						if err != nil {
							return fmt.Errorf("muxer error: %w", err)
						}

						return nil
					})
				}
			}
		}
	}
}

// FromStream maps a MediaMTX stream to a HLS muxer.
func FromStream(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	muxer *gohlslib.Muxer,
	l logger.Writer,
) error {
	setuppedFormats := make(map[format.Format]struct{})

	setupVideoTrack(
		stream,
		writer,
		muxer,
		setuppedFormats,
	)

	setupAudioTracks(
		stream,
		writer,
		muxer,
		setuppedFormats,
	)

	if len(muxer.Tracks) == 0 {
		return ErrNoSupportedCodecs
	}

	n := 1
	for _, media := range stream.Desc().Medias {
		for _, forma := range media.Formats {
			if _, ok := setuppedFormats[forma]; !ok {
				l.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	return nil
}
