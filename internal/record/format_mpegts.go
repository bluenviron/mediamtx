package record

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"time"

	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	mpegtsMaxBufferSize = 64 * 1024
)

func durationGoToMPEGTS(v time.Duration) int64 {
	return int64(v.Seconds() * 90000)
}

type dynamicWriter struct {
	w io.Writer
}

func (d *dynamicWriter) Write(p []byte) (int, error) {
	return d.w.Write(p)
}

func (d *dynamicWriter) setTarget(w io.Writer) {
	d.w = w
}

type formatMPEGTS struct {
	a *agentInstance

	dw             *dynamicWriter
	bw             *bufio.Writer
	mw             *mpegts.Writer
	hasVideo       bool
	currentSegment *formatMPEGTSSegment
}

func (f *formatMPEGTS) initialize() {
	var tracks []*mpegts.Track
	var formats []rtspformat.Format

	addTrack := func(format rtspformat.Format, codec mpegts.Codec) *mpegts.Track {
		track := &mpegts.Track{
			Codec: codec,
		}

		tracks = append(tracks, track)
		formats = append(formats, format)
		return track
	}

	for _, media := range f.a.agent.Stream.Desc().Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *rtspformat.H265: //nolint:dupl
				track := addTrack(forma, &mpegts.CodecH265{})

				var dtsExtractor *h265.DTSExtractor

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
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

					return f.write(
						dts,
						tunit.NTP,
						true,
						randomAccess,
						func() error {
							return f.mw.WriteH265(track, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), randomAccess, tunit.AU)
						},
					)
				})

			case *rtspformat.H264: //nolint:dupl
				track := addTrack(forma, &mpegts.CodecH264{})

				var dtsExtractor *h264.DTSExtractor

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.H264)
					if tunit.AU == nil {
						return nil
					}

					randomAccess := h264.IDRPresent(tunit.AU)

					if dtsExtractor == nil {
						if !randomAccess {
							return nil
						}
						dtsExtractor = h264.NewDTSExtractor()
					}

					dts, err := dtsExtractor.Extract(tunit.AU, tunit.PTS)
					if err != nil {
						return err
					}

					return f.write(
						dts,
						tunit.NTP,
						true,
						randomAccess,
						func() error {
							return f.mw.WriteH264(track, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), randomAccess, tunit.AU)
						},
					)
				})

			case *rtspformat.MPEG4Video:
				track := addTrack(forma, &mpegts.CodecMPEG4Video{})

				firstReceived := false
				var lastPTS time.Duration

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
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

					randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})

					return f.write(
						tunit.PTS,
						tunit.NTP,
						true,
						randomAccess,
						func() error {
							return f.mw.WriteMPEG4Video(track, durationGoToMPEGTS(tunit.PTS), tunit.Frame)
						},
					)
				})

			case *rtspformat.MPEG1Video:
				track := addTrack(forma, &mpegts.CodecMPEG1Video{})

				firstReceived := false
				var lastPTS time.Duration

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
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

					randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, 0xB8})

					return f.write(
						tunit.PTS,
						tunit.NTP,
						true,
						randomAccess,
						func() error {
							return f.mw.WriteMPEG1Video(track, durationGoToMPEGTS(tunit.PTS), tunit.Frame)
						},
					)
				})

			case *rtspformat.Opus:
				track := addTrack(forma, &mpegts.CodecOpus{
					ChannelCount: forma.ChannelCount,
				})

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.Opus)
					if tunit.Packets == nil {
						return nil
					}

					return f.write(
						tunit.PTS,
						tunit.NTP,
						false,
						true,
						func() error {
							return f.mw.WriteOpus(track, durationGoToMPEGTS(tunit.PTS), tunit.Packets)
						},
					)
				})

			case *rtspformat.MPEG4Audio:
				track := addTrack(forma, &mpegts.CodecMPEG4Audio{
					Config: *forma.GetConfig(),
				})

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG4Audio)
					if tunit.AUs == nil {
						return nil
					}

					return f.write(
						tunit.PTS,
						tunit.NTP,
						false,
						true,
						func() error {
							return f.mw.WriteMPEG4Audio(track, durationGoToMPEGTS(tunit.PTS), tunit.AUs)
						},
					)
				})

			case *rtspformat.MPEG1Audio:
				track := addTrack(forma, &mpegts.CodecMPEG1Audio{})

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG1Audio)
					if tunit.Frames == nil {
						return nil
					}

					return f.write(
						tunit.PTS,
						tunit.NTP,
						false,
						true,
						func() error {
							return f.mw.WriteMPEG1Audio(track, durationGoToMPEGTS(tunit.PTS), tunit.Frames)
						},
					)
				})

			case *rtspformat.AC3:
				track := addTrack(forma, &mpegts.CodecAC3{})

				sampleRate := time.Duration(forma.SampleRate)

				f.a.agent.Stream.AddReader(f.a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.AC3)
					if tunit.Frames == nil {
						return nil
					}

					return f.write(
						tunit.PTS,
						tunit.NTP,
						false,
						true,
						func() error {
							for i, frame := range tunit.Frames {
								framePTS := tunit.PTS + time.Duration(i)*ac3.SamplesPerFrame*
									time.Second/sampleRate

								err := f.mw.WriteAC3(track, durationGoToMPEGTS(framePTS), frame)
								if err != nil {
									return err
								}
							}

							return nil
						},
					)
				})
			}
		}
	}

	f.dw = &dynamicWriter{}
	f.bw = bufio.NewWriterSize(f.dw, mpegtsMaxBufferSize)
	f.mw = mpegts.NewWriter(f.bw, tracks)

	f.a.agent.Log(logger.Info, "recording %s",
		defs.FormatsInfo(formats))
}

func (f *formatMPEGTS) close() {
	if f.currentSegment != nil {
		f.currentSegment.close() //nolint:errcheck
	}
}

func (f *formatMPEGTS) write(
	dts time.Duration,
	ntp time.Time,
	isVideo bool,
	randomAccess bool,
	writeCB func() error,
) error {
	if isVideo {
		f.hasVideo = true
	}

	switch {
	case f.currentSegment == nil:
		f.currentSegment = &formatMPEGTSSegment{
			f:        f,
			startDTS: dts,
			startNTP: ntp,
		}
		f.currentSegment.initialize()
	case (!f.hasVideo || isVideo) &&
		randomAccess &&
		(dts-f.currentSegment.startDTS) >= f.a.agent.SegmentDuration:
		f.currentSegment.lastDTS = dts
		err := f.currentSegment.close()
		if err != nil {
			return err
		}

		f.currentSegment = &formatMPEGTSSegment{
			f:        f,
			startDTS: dts,
			startNTP: ntp,
		}
		f.currentSegment.initialize()

	case (dts - f.currentSegment.lastFlush) >= f.a.agent.PartDuration:
		err := f.bw.Flush()
		if err != nil {
			return err
		}

		f.currentSegment.lastFlush = dts
	}

	f.currentSegment.lastDTS = dts

	return writeCB()
}
