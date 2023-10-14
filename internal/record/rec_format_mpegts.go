package record

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

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

type recFormatMPEGTS struct {
	a *Agent

	dw             *dynamicWriter
	bw             *bufio.Writer
	mw             *mpegts.Writer
	hasVideo       bool
	currentSegment *recFormatMPEGTSSegment
}

func newRecFormatMPEGTS(a *Agent) recFormat {
	f := &recFormatMPEGTS{
		a: a,
	}

	var tracks []*mpegts.Track

	addTrack := func(codec mpegts.Codec) *mpegts.Track {
		track := &mpegts.Track{
			Codec: codec,
		}
		tracks = append(tracks, track)
		return track
	}

	for _, media := range a.stream.Desc().Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.H265:
				track := addTrack(&mpegts.CodecH265{})

				var dtsExtractor *h265.DTSExtractor

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
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

					return f.recordH26x(track, dts, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), randomAccess, tunit.AU)
				})

			case *format.H264:
				track := addTrack(&mpegts.CodecH264{})

				var dtsExtractor *h264.DTSExtractor

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
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

					return f.recordH26x(track, dts, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), idrPresent, tunit.AU)
				})

			case *format.MPEG4Video:
				track := addTrack(&mpegts.CodecMPEG4Video{})

				firstReceived := false
				var lastPTS time.Duration

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
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

					f.hasVideo = true
					randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})

					err := f.setupSegment(tunit.PTS, true, randomAccess)
					if err != nil {
						return err
					}

					return f.mw.WriteMPEG4Video(track, durationGoToMPEGTS(tunit.PTS), tunit.Frame)
				})

			case *format.MPEG1Video:
				track := addTrack(&mpegts.CodecMPEG1Video{})

				firstReceived := false
				var lastPTS time.Duration

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
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

					f.hasVideo = true
					randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, 0xB8})

					err := f.setupSegment(tunit.PTS, true, randomAccess)
					if err != nil {
						return err
					}

					return f.mw.WriteMPEG1Video(track, durationGoToMPEGTS(tunit.PTS), tunit.Frame)
				})

			case *format.Opus:
				track := addTrack(&mpegts.CodecOpus{
					ChannelCount: func() int {
						if forma.IsStereo {
							return 2
						}
						return 1
					}(),
				})

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.Opus)
					if tunit.Packets == nil {
						return nil
					}

					err := f.setupSegment(tunit.PTS, false, true)
					if err != nil {
						return err
					}

					return f.mw.WriteOpus(track, durationGoToMPEGTS(tunit.PTS), tunit.Packets)
				})

			case *format.MPEG4Audio:
				track := addTrack(&mpegts.CodecMPEG4Audio{
					Config: *forma.GetConfig(),
				})

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG4Audio)
					if tunit.AUs == nil {
						return nil
					}

					err := f.setupSegment(tunit.PTS, false, true)
					if err != nil {
						return err
					}

					return f.mw.WriteMPEG4Audio(track, durationGoToMPEGTS(tunit.PTS), tunit.AUs)
				})

			case *format.MPEG1Audio:
				track := addTrack(&mpegts.CodecMPEG1Audio{})

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG1Audio)
					if tunit.Frames == nil {
						return nil
					}

					err := f.setupSegment(tunit.PTS, false, true)
					if err != nil {
						return err
					}

					return f.mw.WriteMPEG1Audio(track, durationGoToMPEGTS(tunit.PTS), tunit.Frames)
				})

			case *format.AC3:
				track := addTrack(&mpegts.CodecAC3{})

				sampleRate := time.Duration(forma.SampleRate)

				a.stream.AddReader(a.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.AC3)
					if tunit.Frames == nil {
						return nil
					}

					for i, frame := range tunit.Frames {
						framePTS := tunit.PTS + time.Duration(i)*ac3.SamplesPerFrame*
							time.Second/sampleRate

						err := f.mw.WriteAC3(track, durationGoToMPEGTS(framePTS), frame)
						if err != nil {
							return err
						}
					}

					return nil
				})
			}
		}
	}

	f.dw = &dynamicWriter{}
	f.bw = bufio.NewWriterSize(f.dw, mpegtsMaxBufferSize)
	f.mw = mpegts.NewWriter(f.bw, tracks)

	a.Log(logger.Info, "recording %d %s",
		len(tracks),
		func() string {
			if len(tracks) == 1 {
				return "track"
			}
			return "tracks"
		}())

	return f
}

func (f *recFormatMPEGTS) close() {
	if f.currentSegment != nil {
		f.currentSegment.close() //nolint:errcheck
	}
}

func (f *recFormatMPEGTS) setupSegment(dts time.Duration, isVideo bool, randomAccess bool) error {
	switch {
	case f.currentSegment == nil:
		f.currentSegment = newRecFormatMPEGTSSegment(f, dts)

	case (!f.hasVideo || isVideo) &&
		randomAccess &&
		(dts-f.currentSegment.startDTS) >= f.a.segmentDuration:
		err := f.currentSegment.close()
		if err != nil {
			return err
		}

		f.currentSegment = newRecFormatMPEGTSSegment(f, dts)

	case (dts - f.currentSegment.lastFlush) >= f.a.partDuration:
		err := f.bw.Flush()
		if err != nil {
			return err
		}

		f.currentSegment.lastFlush = dts
	}

	return nil
}

func (f *recFormatMPEGTS) recordH26x(track *mpegts.Track, goDTS time.Duration,
	pts int64, dts int64, randomAccess bool, au [][]byte,
) error {
	f.hasVideo = true

	err := f.setupSegment(goDTS, true, randomAccess)
	if err != nil {
		return err
	}

	return f.mw.WriteH26x(track, pts, dts, randomAccess, au)
}
