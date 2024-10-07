// Package hls contains HLS utilities.
package hls

import (
	"errors"
	"fmt"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// ErrNoSupportedCodecs is returned by FromStream when there are no supported codecs.
var ErrNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently AV1, VP9, H265, H264, Opus, MPEG-4 Audio")

func setupVideoTrack(
	strea *stream.Stream,
	reader stream.Reader,
	muxer *gohlslib.Muxer,
	setuppedFormats map[format.Format]struct{},
) {
	addTrack := func(
		media *description.Media,
		forma format.Format,
		track *gohlslib.Track,
		readFunc stream.ReadFunc,
	) {
		muxer.Tracks = append(muxer.Tracks, track)
		setuppedFormats[forma] = struct{}{}
		strea.AddReader(reader, media, forma, readFunc)
	}

	var videoFormatAV1 *format.AV1
	videoMedia := strea.Desc().FindFormat(&videoFormatAV1)

	if videoFormatAV1 != nil {
		track := &gohlslib.Track{
			Codec:     &codecs.AV1{},
			ClockRate: videoFormatAV1.ClockRate(),
		}

		addTrack(
			videoMedia,
			videoFormatAV1,
			track,
			func(u unit.Unit) error {
				tunit := u.(*unit.AV1)

				if tunit.TU == nil {
					return nil
				}

				err := muxer.WriteAV1(
					track,
					tunit.NTP,
					tunit.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					tunit.TU)
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}

	var videoFormatVP9 *format.VP9
	videoMedia = strea.Desc().FindFormat(&videoFormatVP9)

	if videoFormatVP9 != nil {
		track := &gohlslib.Track{
			Codec:     &codecs.VP9{},
			ClockRate: videoFormatVP9.ClockRate(),
		}

		addTrack(
			videoMedia,
			videoFormatVP9,
			track,
			func(u unit.Unit) error {
				tunit := u.(*unit.VP9)

				if tunit.Frame == nil {
					return nil
				}

				err := muxer.WriteVP9(
					track,
					tunit.NTP,
					tunit.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					tunit.Frame)
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}

	var videoFormatH265 *format.H265
	videoMedia = strea.Desc().FindFormat(&videoFormatH265)

	if videoFormatH265 != nil {
		vps, sps, pps := videoFormatH265.SafeParams()
		track := &gohlslib.Track{
			Codec: &codecs.H265{
				VPS: vps,
				SPS: sps,
				PPS: pps,
			},
			ClockRate: videoFormatH265.ClockRate(),
		}

		addTrack(
			videoMedia,
			videoFormatH265,
			track,
			func(u unit.Unit) error {
				tunit := u.(*unit.H265)

				if tunit.AU == nil {
					return nil
				}

				err := muxer.WriteH265(
					track,
					tunit.NTP,
					tunit.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					tunit.AU)
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}

	var videoFormatH264 *format.H264
	videoMedia = strea.Desc().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		sps, pps := videoFormatH264.SafeParams()
		track := &gohlslib.Track{
			Codec: &codecs.H264{
				SPS: sps,
				PPS: pps,
			},
			ClockRate: videoFormatH264.ClockRate(),
		}

		addTrack(
			videoMedia,
			videoFormatH264,
			track,
			func(u unit.Unit) error {
				tunit := u.(*unit.H264)

				if tunit.AU == nil {
					return nil
				}

				err := muxer.WriteH264(
					track,
					tunit.NTP,
					tunit.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					tunit.AU)
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}
}

func setupAudioTracks(
	strea *stream.Stream,
	reader stream.Reader,
	muxer *gohlslib.Muxer,
	setuppedFormats map[format.Format]struct{},
) {
	addTrack := func(
		medi *description.Media,
		forma format.Format,
		track *gohlslib.Track,
		readFunc stream.ReadFunc,
	) {
		muxer.Tracks = append(muxer.Tracks, track)
		setuppedFormats[forma] = struct{}{}
		strea.AddReader(reader, medi, forma, readFunc)
	}

	for _, media := range strea.Desc().Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.Opus:
				track := &gohlslib.Track{
					Codec: &codecs.Opus{
						ChannelCount: forma.ChannelCount,
					},
					ClockRate: forma.ClockRate(),
				}

				addTrack(
					media,
					forma,
					track,
					func(u unit.Unit) error {
						tunit := u.(*unit.Opus)

						err := muxer.WriteOpus(
							track,
							tunit.NTP,
							tunit.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
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
						ClockRate: forma.ClockRate(),
					}

					addTrack(
						media,
						forma,
						track,
						func(u unit.Unit) error {
							tunit := u.(*unit.MPEG4Audio)

							if tunit.AUs == nil {
								return nil
							}

							err := muxer.WriteMPEG4Audio(
								track,
								tunit.NTP,
								tunit.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
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
	reader stream.Reader,
	muxer *gohlslib.Muxer,
) error {
	setuppedFormats := make(map[format.Format]struct{})

	setupVideoTrack(
		stream,
		reader,
		muxer,
		setuppedFormats,
	)

	setupAudioTracks(
		stream,
		reader,
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
				reader.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	return nil
}
