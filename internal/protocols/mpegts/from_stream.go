package mpegts

import (
	"bufio"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	mcmpegts "github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func durationGoToMPEGTS(v time.Duration) int64 {
	return int64(v.Seconds() * 90000)
}

// FromStream links a server stream to a MPEG-TS writer.
func FromStream(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	bw *bufio.Writer,
	sconn srt.Conn,
	writeTimeout time.Duration,
) error {
	var w *mcmpegts.Writer
	var tracks []*mcmpegts.Track

	addTrack := func(codec mcmpegts.Codec) *mcmpegts.Track {
		track := &mcmpegts.Track{
			Codec: codec,
		}
		tracks = append(tracks, track)
		return track
	}

	for _, medi := range stream.Desc().Medias {
		for _, forma := range medi.Formats {
			switch forma := forma.(type) {
			case *format.H265: //nolint:dupl
				track := addTrack(&mcmpegts.CodecH265{})

				var dtsExtractor *h265.DTSExtractor

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.H265)
					if tunit.AU == nil {
						return nil
					}

					randomAccess := h265.IsRandomAccess(tunit.AU)

					if dtsExtractor == nil {
						if !randomAccess {
							return nil
						}
						dtsExtractor = h265.NewDTSExtractor()
					}

					dts, err := dtsExtractor.Extract(tunit.AU, tunit.PTS)
					if err != nil {
						return err
					}

					sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err = (*w).WriteH265(track, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), randomAccess, tunit.AU)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.H264: //nolint:dupl
				track := addTrack(&mcmpegts.CodecH264{})

				var dtsExtractor *h264.DTSExtractor

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.H264)
					if tunit.AU == nil {
						return nil
					}

					idrPresent := h264.IDRPresent(tunit.AU)

					if dtsExtractor == nil {
						if !idrPresent {
							return nil
						}
						dtsExtractor = h264.NewDTSExtractor()
					}

					dts, err := dtsExtractor.Extract(tunit.AU, tunit.PTS)
					if err != nil {
						return err
					}

					sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err = (*w).WriteH264(track, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), idrPresent, tunit.AU)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.MPEG4Video:
				track := addTrack(&mcmpegts.CodecMPEG4Video{})

				firstReceived := false
				var lastPTS time.Duration

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG4Video)
					if tunit.Frame == nil {
						return nil
					}

					if !firstReceived {
						firstReceived = true
					} else if tunit.PTS < lastPTS {
						return fmt.Errorf("MPEG-4 Video streams with B-frames are not supported (yet)")
					}
					lastPTS = tunit.PTS

					sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err := (*w).WriteMPEG4Video(track, durationGoToMPEGTS(tunit.PTS), tunit.Frame)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.MPEG1Video:
				track := addTrack(&mcmpegts.CodecMPEG1Video{})

				firstReceived := false
				var lastPTS time.Duration

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG1Video)
					if tunit.Frame == nil {
						return nil
					}

					if !firstReceived {
						firstReceived = true
					} else if tunit.PTS < lastPTS {
						return fmt.Errorf("MPEG-1 Video streams with B-frames are not supported (yet)")
					}
					lastPTS = tunit.PTS

					sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err := (*w).WriteMPEG1Video(track, durationGoToMPEGTS(tunit.PTS), tunit.Frame)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.Opus:
				track := addTrack(&mcmpegts.CodecOpus{
					ChannelCount: forma.ChannelCount,
				})

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.Opus)
					if tunit.Packets == nil {
						return nil
					}

					sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err := (*w).WriteOpus(track, durationGoToMPEGTS(tunit.PTS), tunit.Packets)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.MPEG4Audio:
				track := addTrack(&mcmpegts.CodecMPEG4Audio{
					Config: *forma.GetConfig(),
				})

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG4Audio)
					if tunit.AUs == nil {
						return nil
					}

					sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err := (*w).WriteMPEG4Audio(track, durationGoToMPEGTS(tunit.PTS), tunit.AUs)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.MPEG1Audio:
				track := addTrack(&mcmpegts.CodecMPEG1Audio{})

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG1Audio)
					if tunit.Frames == nil {
						return nil
					}

					sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err := (*w).WriteMPEG1Audio(track, durationGoToMPEGTS(tunit.PTS), tunit.Frames)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.AC3:
				track := addTrack(&mcmpegts.CodecAC3{})

				sampleRate := time.Duration(forma.SampleRate)

				stream.AddReader(writer, medi, forma, func(u unit.Unit) error {
					tunit := u.(*unit.AC3)
					if tunit.Frames == nil {
						return nil
					}

					for i, frame := range tunit.Frames {
						framePTS := tunit.PTS + time.Duration(i)*ac3.SamplesPerFrame*
							time.Second/sampleRate

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteAC3(track, durationGoToMPEGTS(framePTS), frame)
						if err != nil {
							return err
						}
					}
					return bw.Flush()
				})
			}
		}
	}

	if len(tracks) == 0 {
		return ErrNoTracks
	}

	w = mcmpegts.NewWriter(bw, tracks)

	return nil
}
