// Package rtmp provides RTMP utilities.
package rtmp

import (
	"encoding/hex"
	"errors"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/flac"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/bluenviron/mediamtx/internal/formatlabel"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecsFrom = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"AV1, VP9, H265, H264, Opus, FLAC, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711, LPCM")

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func timestampToDuration(t int64, clockRate int) time.Duration {
	return multiplyAndDivide2(time.Duration(t), time.Second, time.Duration(clockRate))
}

// FromStream maps a MediaMTX stream to a RTMP stream.
func FromStream(
	origDesc *description.Session,
	outDesc *description.Session,
	r *stream.Reader,
	conn *gortmplib.ServerConn,
	nconn net.Conn,
	writeTimeout time.Duration,
) error {
	var tracks []*gortmplib.Track
	var w *gortmplib.Writer

	isEnhanced := len(conn.FourCcList) != 0
	legacyVideoTrackCount := 0
	legacyAudioTrackCount := 0

	for i, origMedia := range origDesc.Medias {
		for j, origFormat := range origMedia.Formats {
			switch origFormat := origFormat.(type) {
			case *format.AV1:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCAV1))) {
					track := &gortmplib.Track{
						Codec: &codecs.AV1{},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteAV1(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								u.Payload.(unit.PayloadAV1))
						})
				}

			case *format.VP9:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCVP9))) {
					track := &gortmplib.Track{
						Codec: &codecs.VP9{},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteVP9(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								u.Payload.(unit.PayloadVP9))
						})
				}

			case *format.H265:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCHEVC))) {
					outFormat := outDesc.Medias[i].Formats[j].(*format.H265)

					track := &gortmplib.Track{
						Codec: &codecs.H265{
							VPS: outFormat.VPS,
							SPS: outFormat.SPS,
							PPS: outFormat.PPS,
						},
					}
					tracks = append(tracks, track)

					var videoDTSExtractor *h265.DTSExtractor

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							if videoDTSExtractor == nil {
								if !h265.IsRandomAccess(u.Payload.(unit.PayloadH265)) {
									return nil
								}
								videoDTSExtractor = &h265.DTSExtractor{}
								videoDTSExtractor.Initialize()
							}

							dts, err := videoDTSExtractor.Extract(u.Payload.(unit.PayloadH265), u.PTS)
							if err != nil {
								return err
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteH265(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								timestampToDuration(dts, origFormat.ClockRate()),
								u.Payload.(unit.PayloadH265))
						})
				}

			case *format.H264:
				if isEnhanced || legacyVideoTrackCount == 0 {
					legacyVideoTrackCount++

					outFormat := outDesc.Medias[i].Formats[j].(*format.H264)

					track := &gortmplib.Track{
						Codec: &codecs.H264{
							SPS: outFormat.SPS,
							PPS: outFormat.PPS,
						},
					}
					tracks = append(tracks, track)

					var videoDTSExtractor *h264.DTSExtractor

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							idrPresent := false
							nonIDRPresent := false

							for _, nalu := range u.Payload.(unit.PayloadH264) {
								typ := h264.NALUType(nalu[0] & 0x1F)
								switch typ {
								case h264.NALUTypeIDR:
									idrPresent = true

								case h264.NALUTypeNonIDR:
									nonIDRPresent = true
								}
							}

							// wait until we receive an IDR
							if videoDTSExtractor == nil {
								if !idrPresent {
									return nil
								}

								videoDTSExtractor = &h264.DTSExtractor{}
								videoDTSExtractor.Initialize()
							} else if !idrPresent && !nonIDRPresent {
								return nil
							}

							dts, err := videoDTSExtractor.Extract(u.Payload.(unit.PayloadH264), u.PTS)
							if err != nil {
								return err
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteH264(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								timestampToDuration(dts, origFormat.ClockRate()),
								u.Payload.(unit.PayloadH264))
						})
				}

			case *format.Opus:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCOpus))) {
					track := &gortmplib.Track{
						Codec: &codecs.Opus{
							IDHeader: &opus.IDHeader{
								Version:      0x1,
								ChannelCount: uint8(origFormat.ChannelCount),
								PreSkip:      3840,
							},
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							pts := u.PTS

							for _, pkt := range u.Payload.(unit.PayloadOpus) {
								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err := (*w).WriteOpus(
									track,
									timestampToDuration(pts, origFormat.ClockRate()),
									pkt,
								)
								if err != nil {
									return err
								}

								pts += opus.PacketDuration2(pkt)
							}

							return nil
						})
				}

			case *format.MPEG4Audio:
				if isEnhanced || legacyAudioTrackCount == 0 {
					legacyAudioTrackCount++
					track := &gortmplib.Track{
						Codec: &codecs.MPEG4Audio{
							Config: origFormat.Config,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							for i, au := range u.Payload.(unit.PayloadMPEG4Audio) {
								pts := u.PTS + int64(i)*mpeg4audio.SamplesPerAccessUnit

								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err := (*w).WriteMPEG4Audio(
									track,
									timestampToDuration(pts, origFormat.ClockRate()),
									au,
								)
								if err != nil {
									return err
								}
							}

							return nil
						})
				}

			case *format.MPEG4AudioLATM:
				if !origFormat.CPresent && (isEnhanced || legacyAudioTrackCount == 0) {
					legacyAudioTrackCount++
					track := &gortmplib.Track{
						Codec: &codecs.MPEG4Audio{
							Config: origFormat.StreamMuxConfig.Programs[0].Layers[0].AudioSpecificConfig,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							var ame mpeg4audio.AudioMuxElement
							ame.StreamMuxConfig = origFormat.StreamMuxConfig
							err := ame.Unmarshal(u.Payload.(unit.PayloadMPEG4AudioLATM))
							if err != nil {
								return err
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteMPEG4Audio(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								ame.Payloads[0][0][0],
							)
						})
				}

			case *format.MPEG1Audio:
				if isEnhanced || legacyAudioTrackCount == 0 {
					legacyAudioTrackCount++
					track := &gortmplib.Track{
						Codec: &codecs.MPEG1Audio{},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							pts := u.PTS

							for _, frame := range u.Payload.(unit.PayloadMPEG1Audio) {
								var h mpeg1audio.FrameHeader
								err := h.Unmarshal(frame)
								if err != nil {
									return err
								}

								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err = (*w).WriteMPEG1Audio(
									track,
									timestampToDuration(pts, origFormat.ClockRate()),
									frame)
								if err != nil {
									return err
								}

								pts += int64(h.SampleCount()) *
									int64(origFormat.ClockRate()) / int64(h.SampleRate)
							}

							return nil
						})
				}

			case *format.AC3:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCAC3))) {
					track := &gortmplib.Track{
						Codec: &codecs.AC3{
							SampleRate:   origFormat.SampleRate,
							ChannelCount: origFormat.ChannelCount,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							for i, frame := range u.Payload.(unit.PayloadAC3) {
								pts := u.PTS + int64(i)*ac3.SamplesPerFrame

								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err := (*w).WriteAC3(
									track,
									timestampToDuration(pts, origFormat.ClockRate()),
									frame)
								if err != nil {
									return err
								}
							}

							return nil
						})
				}

			case *format.G711:
				if origFormat.SampleRate == 8000 && (isEnhanced || legacyAudioTrackCount == 0) {
					legacyAudioTrackCount++
					track := &gortmplib.Track{
						Codec: &codecs.G711{
							MULaw:        origFormat.MULaw,
							ChannelCount: origFormat.ChannelCount,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteG711(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								u.Payload.(unit.PayloadG711),
							)
						})
				}

			case *format.LPCM:
				if (origFormat.ChannelCount == 1 || origFormat.ChannelCount == 2) &&
					(origFormat.SampleRate == 5512 ||
						origFormat.SampleRate == 11025 ||
						origFormat.SampleRate == 22050 ||
						origFormat.SampleRate == 44100) &&
					(isEnhanced || legacyAudioTrackCount == 0) {
					legacyAudioTrackCount++
					track := &gortmplib.Track{
						Codec: &codecs.LPCM{
							BitDepth:     origFormat.BitDepth,
							SampleRate:   origFormat.SampleRate,
							ChannelCount: origFormat.ChannelCount,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteLPCM(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								u.Payload.(unit.PayloadLPCM),
							)
						})
				}

			case *format.Generic:
				if strings.HasPrefix(strings.ToLower(origFormat.RTPMap()), "flac/") &&
					slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCFLAC))) {
					enc, err := hex.DecodeString(origFormat.FMT["streaminfo"])
					if err != nil {
						return err
					}

					var streamInfo flac.StreamInfo
					err = streamInfo.Unmarshal(enc)
					if err != nil {
						return err
					}

					track := &gortmplib.Track{
						Codec: &codecs.FLAC{
							StreamInfo: &streamInfo,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						origMedia,
						origFormat,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteFLAC(
								track,
								timestampToDuration(u.PTS, origFormat.ClockRate()),
								u.Payload.(unit.PayloadFLAC),
							)
						})
				}
			}
		}
	}

	if len(tracks) == 0 {
		return errNoSupportedCodecsFrom
	}

	w = &gortmplib.Writer{
		Conn:   conn,
		Tracks: tracks,
	}
	err := w.Initialize()
	if err != nil {
		return err
	}

	setuppedFormats := r.Formats()

	n := 1
	for _, origMedia := range origDesc.Medias {
		for _, origFormat := range origMedia.Formats {
			if !slices.Contains(setuppedFormats, origFormat) {
				r.Parent.Log(logger.Warn, "skipping track %d (%s)", n, formatlabel.FormatToLabel(origFormat))
			}
			n++
		}
	}

	return nil
}
