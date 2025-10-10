// Package hls contains HLS utilities.
package hls

import (
	"errors"
	"fmt"
	"slices"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// ErrNoSupportedCodecs is returned by FromStream when there are no supported codecs.
var ErrNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently AV1, VP9, H265, H264, Opus, MPEG-4 Audio")

func setupVideoTrack(
	desc *description.Session,
	r *stream.Reader,
	muxer *gohlslib.Muxer,
) {
	addTrack := func(
		media *description.Media,
		forma format.Format,
		track *gohlslib.Track,
		onData stream.OnDataFunc,
	) {
		muxer.Tracks = append(muxer.Tracks, track)
		r.OnData(media, forma, onData)
	}

	var videoFormatAV1 *format.AV1
	videoMedia := desc.FindFormat(&videoFormatAV1)

	if videoFormatAV1 != nil {
		track := &gohlslib.Track{
			Codec:     &codecs.AV1{},
			ClockRate: videoFormatAV1.ClockRate(),
		}

		addTrack(
			videoMedia,
			videoFormatAV1,
			track,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				err := muxer.WriteAV1(
					track,
					u.NTP,
					u.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					u.Payload.(unit.PayloadAV1))
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}

	var videoFormatVP9 *format.VP9
	videoMedia = desc.FindFormat(&videoFormatVP9)

	if videoFormatVP9 != nil {
		track := &gohlslib.Track{
			Codec:     &codecs.VP9{},
			ClockRate: videoFormatVP9.ClockRate(),
		}

		addTrack(
			videoMedia,
			videoFormatVP9,
			track,
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				err := muxer.WriteVP9(
					track,
					u.NTP,
					u.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					u.Payload.(unit.PayloadVP9))
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}

	var videoFormatH265 *format.H265
	videoMedia = desc.FindFormat(&videoFormatH265)

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
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				err := muxer.WriteH265(
					track,
					u.NTP,
					u.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					u.Payload.(unit.PayloadH265))
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}

	var videoFormatH264 *format.H264
	videoMedia = desc.FindFormat(&videoFormatH264)

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
			func(u *unit.Unit) error {
				if u.NilPayload() {
					return nil
				}

				err := muxer.WriteH264(
					track,
					u.NTP,
					u.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
					u.Payload.(unit.PayloadH264))
				if err != nil {
					return fmt.Errorf("muxer error: %w", err)
				}

				return nil
			})

		return
	}
}

func setupAudioTracks(
	desc *description.Session,
	r *stream.Reader,
	muxer *gohlslib.Muxer,
) {
	addTrack := func(
		medi *description.Media,
		forma format.Format,
		track *gohlslib.Track,
		onData stream.OnDataFunc,
	) {
		muxer.Tracks = append(muxer.Tracks, track)
		r.OnData(medi, forma, onData)
	}

	for _, media := range desc.Medias {
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
					func(u *unit.Unit) error {
						err := muxer.WriteOpus(
							track,
							u.NTP,
							u.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
							u.Payload.(unit.PayloadOpus))
						if err != nil {
							return fmt.Errorf("muxer error: %w", err)
						}

						return nil
					})

			case *format.MPEG4Audio:
				track := &gohlslib.Track{
					Codec: &codecs.MPEG4Audio{
						Config: *forma.Config,
					},
					ClockRate: forma.ClockRate(),
				}

				addTrack(
					media,
					forma,
					track,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						err := muxer.WriteMPEG4Audio(
							track,
							u.NTP,
							u.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
							u.Payload.(unit.PayloadMPEG4Audio))
						if err != nil {
							return fmt.Errorf("muxer error: %w", err)
						}

						return nil
					})

			case *format.MPEG4AudioLATM:
				if !forma.CPresent {
					track := &gohlslib.Track{
						Codec: &codecs.MPEG4Audio{
							Config: *forma.StreamMuxConfig.Programs[0].Layers[0].AudioSpecificConfig,
						},
						ClockRate: forma.ClockRate(),
					}

					addTrack(
						media,
						forma,
						track,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							var ame mpeg4audio.AudioMuxElement
							ame.StreamMuxConfig = forma.StreamMuxConfig
							err := ame.Unmarshal(u.Payload.(unit.PayloadMPEG4AudioLATM))
							if err != nil {
								return err
							}

							return muxer.WriteMPEG4Audio(
								track,
								u.NTP,
								u.PTS, // no conversion is needed since we set gohlslib.Track.ClockRate = format.ClockRate
								[][]byte{ame.Payloads[0][0][0]})
						})
				}
			}
		}
	}
}

// FromStream maps a MediaMTX stream to a HLS muxer.
func FromStream(
	desc *description.Session,
	r *stream.Reader,
	muxer *gohlslib.Muxer,
) error {
	setupVideoTrack(
		desc,
		r,
		muxer,
	)

	setupAudioTracks(
		desc,
		r,
		muxer,
	)

	if len(muxer.Tracks) == 0 {
		return ErrNoSupportedCodecs
	}

	setuppedFormats := r.Formats()

	n := 1
	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			if !slices.Contains(setuppedFormats, forma) {
				r.Parent.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	return nil
}
