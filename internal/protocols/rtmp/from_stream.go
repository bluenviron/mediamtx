// Package rtmp provides RTMP utilities.
package rtmp

import (
	"errors"
	"net"
	"slices"
	"time"

	"github.com/bluenviron/gortmplib"
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
	var tracks []format.Format
	var w *gortmplib.Writer

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.AV1:
				if slices.Contains(conn.FourCcList, interface{}(fourCCToString(message.FourCCAV1))) {
					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.AV1)

							if tunit.TU == nil {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteAV1(
								forma,
								timestampToDuration(tunit.PTS, forma.ClockRate()),
								tunit.TU)
						})

					tracks = append(tracks, forma)
				}

			case *format.VP9:
				if slices.Contains(conn.FourCcList, interface{}(fourCCToString(message.FourCCVP9))) {
					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.VP9)

							if tunit.Frame == nil {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteVP9(
								forma,
								timestampToDuration(tunit.PTS, forma.ClockRate()),
								tunit.Frame)
						})

					tracks = append(tracks, forma)
				}

			case *format.H265:
				if slices.Contains(conn.FourCcList, interface{}(fourCCToString(message.FourCCHEVC))) {
					var videoDTSExtractor *h265.DTSExtractor

					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.H265)

							if tunit.AU == nil {
								return nil
							}

							if videoDTSExtractor == nil {
								if !h265.IsRandomAccess(tunit.AU) {
									return nil
								}
								videoDTSExtractor = &h265.DTSExtractor{}
								videoDTSExtractor.Initialize()
							}

							dts, err := videoDTSExtractor.Extract(tunit.AU, tunit.PTS)
							if err != nil {
								return err
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteH265(
								forma,
								timestampToDuration(tunit.PTS, forma.ClockRate()),
								timestampToDuration(dts, forma.ClockRate()),
								tunit.AU)
						})

					tracks = append(tracks, forma)
				}

			case *format.H264:
				var videoDTSExtractor *h264.DTSExtractor

				r.OnData(
					media,
					forma,
					func(u unit.Unit) error {
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

						dts, err := videoDTSExtractor.Extract(tunit.AU, tunit.PTS)
						if err != nil {
							return err
						}

						nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						return (*w).WriteH264(
							forma,
							timestampToDuration(tunit.PTS, forma.ClockRate()),
							timestampToDuration(dts, forma.ClockRate()),
							tunit.AU)
					})

				tracks = append(tracks, forma)

			case *format.Opus:
				if slices.Contains(conn.FourCcList, interface{}(fourCCToString(message.FourCCOpus))) {
					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.Opus)

							if tunit.Packets == nil {
								return nil
							}

							pts := tunit.PTS

							for _, pkt := range tunit.Packets {
								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err := (*w).WriteOpus(
									forma,
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

					tracks = append(tracks, forma)
				}

			case *format.MPEG4Audio:
				r.OnData(
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.MPEG4Audio)

						if tunit.AUs == nil {
							return nil
						}

						for i, au := range tunit.AUs {
							pts := tunit.PTS + int64(i)*mpeg4audio.SamplesPerAccessUnit

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err := (*w).WriteMPEG4Audio(
								forma,
								timestampToDuration(pts, forma.ClockRate()),
								au,
							)
							if err != nil {
								return err
							}
						}

						return nil
					})

				tracks = append(tracks, forma)

			case *format.MPEG4AudioLATM:
				if !forma.CPresent {
					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.MPEG4AudioLATM)

							if tunit.Element == nil {
								return nil
							}

							var ame mpeg4audio.AudioMuxElement
							ame.StreamMuxConfig = forma.StreamMuxConfig
							err := ame.Unmarshal(tunit.Element)
							if err != nil {
								return err
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteMPEG4Audio(
								forma,
								timestampToDuration(tunit.PTS, forma.ClockRate()),
								ame.Payloads[0][0][0],
							)
						})

					tracks = append(tracks, forma)
				}

			case *format.MPEG1Audio:
				// TODO: check sample rate and layer,
				// unfortunately they are not available at this stage.
				r.OnData(
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.MPEG1Audio)

						pts := tunit.PTS

						for _, frame := range tunit.Frames {
							var h mpeg1audio.FrameHeader
							err := h.Unmarshal(frame)
							if err != nil {
								return err
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err = (*w).WriteMPEG1Audio(
								forma,
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

				tracks = append(tracks, forma)

			case *format.AC3:
				if slices.Contains(conn.FourCcList, interface{}(fourCCToString(message.FourCCAC3))) {
					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.AC3)

							for i, frame := range tunit.Frames {
								pts := tunit.PTS + int64(i)*ac3.SamplesPerFrame

								nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
								err := (*w).WriteAC3(
									forma,
									timestampToDuration(pts, forma.ClockRate()),
									frame)
								if err != nil {
									return err
								}
							}

							return nil
						})

					tracks = append(tracks, forma)
				}

			case *format.G711:
				if forma.SampleRate == 8000 {
					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.G711)

							if tunit.Samples == nil {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteG711(
								forma,
								timestampToDuration(tunit.PTS, forma.ClockRate()),
								tunit.Samples,
							)
						})

					tracks = append(tracks, forma)
				}

			case *format.LPCM:
				if (forma.ChannelCount == 1 || forma.ChannelCount == 2) &&
					(forma.SampleRate == 5512 ||
						forma.SampleRate == 11025 ||
						forma.SampleRate == 22050 ||
						forma.SampleRate == 44100) {
					r.OnData(
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.LPCM)

							if tunit.Samples == nil {
								return nil
							}

							nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							return (*w).WriteLPCM(
								forma,
								timestampToDuration(tunit.PTS, forma.ClockRate()),
								tunit.Samples,
							)
						})

					tracks = append(tracks, forma)
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

	n := 1
	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			if !slices.Contains(tracks, forma) {
				r.Parent.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	return nil
}
