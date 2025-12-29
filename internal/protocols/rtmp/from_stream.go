// Package rtmp provides RTMP utilities.
package rtmp

import (
	"errors"
	"net"
	"slices"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecsFrom = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"AV1, VP9, H265, H264, Opus, MPEG-4 Audio, MPEG-1/2 Audio, AC-3, G711, LPCM")

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
	desc *description.Session,
	r *stream.Reader,
	conn *gortmplib.ServerConn,
	nconn net.Conn,
	writeTimeout time.Duration,
) error {
	var tracks []*gortmplib.Track
	var w *gortmplib.Writer

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.AV1:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCAV1))) {
					track := &gortmplib.Track{
						Codec: &codecs.AV1{},
					}
					tracks = append(tracks, track)

					r.OnData(
						media,
						forma,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteAV1(
								track,
								timestampToDuration(u.PTS, forma.ClockRate()),
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
						media,
						forma,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteVP9(
								track,
								timestampToDuration(u.PTS, forma.ClockRate()),
								u.Payload.(unit.PayloadVP9))
						})
				}

			case *format.H265:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCHEVC))) {
					vps, sps, pps := forma.SafeParams()
					track := &gortmplib.Track{
						Codec: &codecs.H265{
							VPS: vps,
							SPS: sps,
							PPS: pps,
						},
					}
					tracks = append(tracks, track)

					var videoDTSExtractor *h265.DTSExtractor

					r.OnData(
						media,
						forma,
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
								timestampToDuration(u.PTS, forma.ClockRate()),
								timestampToDuration(dts, forma.ClockRate()),
								u.Payload.(unit.PayloadH265))
						})
				}

			case *format.H264:
				sps, pps := forma.SafeParams()
				track := &gortmplib.Track{
					Codec: &codecs.H264{
						SPS: sps,
						PPS: pps,
					},
				}
				tracks = append(tracks, track)

				var videoDTSExtractor *h264.DTSExtractor

				r.OnData(
					media,
					forma,
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
							timestampToDuration(u.PTS, forma.ClockRate()),
							timestampToDuration(dts, forma.ClockRate()),
							u.Payload.(unit.PayloadH264))
					})

			case *format.Opus:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCOpus))) {
					track := &gortmplib.Track{
						Codec: &codecs.Opus{
							ChannelCount: forma.ChannelCount,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						media,
						forma,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							pts := u.PTS

							for _, pkt := range u.Payload.(unit.PayloadOpus) {
								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err := (*w).WriteOpus(
									track,
									timestampToDuration(pts, forma.ClockRate()),
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
				track := &gortmplib.Track{
					Codec: &codecs.MPEG4Audio{
						Config: forma.Config,
					},
				}
				tracks = append(tracks, track)

				r.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						for i, au := range u.Payload.(unit.PayloadMPEG4Audio) {
							pts := u.PTS + int64(i)*mpeg4audio.SamplesPerAccessUnit

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err := (*w).WriteMPEG4Audio(
								track,
								timestampToDuration(pts, forma.ClockRate()),
								au,
							)
							if err != nil {
								return err
							}
						}

						return nil
					})

			case *format.MPEG4AudioLATM:
				if !forma.CPresent {
					track := &gortmplib.Track{
						Codec: &codecs.MPEG4Audio{
							Config: forma.StreamMuxConfig.Programs[0].Layers[0].AudioSpecificConfig,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						media,
						forma,
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

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteMPEG4Audio(
								track,
								timestampToDuration(u.PTS, forma.ClockRate()),
								ame.Payloads[0][0][0],
							)
						})
				}

			case *format.MPEG1Audio:
				track := &gortmplib.Track{
					Codec: &codecs.MPEG1Audio{},
				}
				tracks = append(tracks, track)

				r.OnData(
					media,
					forma,
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
								timestampToDuration(pts, forma.ClockRate()),
								frame)
							if err != nil {
								return err
							}

							pts += int64(h.SampleCount()) *
								int64(forma.ClockRate()) / int64(h.SampleRate)
						}

						return nil
					})

			case *format.AC3:
				if slices.Contains(conn.FourCcList, any(fourCCToString(message.FourCCAC3))) {
					track := &gortmplib.Track{
						Codec: &codecs.AC3{
							SampleRate:   forma.SampleRate,
							ChannelCount: forma.ChannelCount,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						media,
						forma,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							for i, frame := range u.Payload.(unit.PayloadAC3) {
								pts := u.PTS + int64(i)*ac3.SamplesPerFrame

								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err := (*w).WriteAC3(
									track,
									timestampToDuration(pts, forma.ClockRate()),
									frame)
								if err != nil {
									return err
								}
							}

							return nil
						})
				}

			case *format.G711:
				if forma.SampleRate == 8000 {
					track := &gortmplib.Track{
						Codec: &codecs.G711{
							MULaw:        forma.MULaw,
							SampleRate:   forma.SampleRate,
							ChannelCount: forma.ChannelCount,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						media,
						forma,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteG711(
								track,
								timestampToDuration(u.PTS, forma.ClockRate()),
								u.Payload.(unit.PayloadG711),
							)
						})
				}

			case *format.LPCM:
				if (forma.ChannelCount == 1 || forma.ChannelCount == 2) &&
					(forma.SampleRate == 5512 ||
						forma.SampleRate == 11025 ||
						forma.SampleRate == 22050 ||
						forma.SampleRate == 44100) {
					track := &gortmplib.Track{
						Codec: &codecs.LPCM{
							BitDepth:     forma.BitDepth,
							SampleRate:   forma.SampleRate,
							ChannelCount: forma.ChannelCount,
						},
					}
					tracks = append(tracks, track)

					r.OnData(
						media,
						forma,
						func(u *unit.Unit) error {
							if u.NilPayload() {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteLPCM(
								track,
								timestampToDuration(u.PTS, forma.ClockRate()),
								u.Payload.(unit.PayloadLPCM),
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
