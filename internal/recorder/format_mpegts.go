package recorder

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"slices"
	"time"

	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	mpegtsBufferSize = 64 * 1024
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func timestampToDuration(t int64, clockRate int) time.Duration {
	return multiplyAndDivide2(time.Duration(t), time.Second, time.Duration(clockRate))
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
	ri *recorderInstance

	dw             *dynamicWriter
	bw             *bufio.Writer
	mw             *mpegts.Writer
	hasVideo       bool
	currentSegment *formatMPEGTSSegment
}

func (f *formatMPEGTS) initialize() bool {
	var tracks []*mpegts.Track

	addTrack := func(codec mpegts.Codec) *mpegts.Track {
		track := &mpegts.Track{
			Codec: codec,
		}
		tracks = append(tracks, track)
		return track
	}

	for _, media := range f.ri.stream.Desc.Medias {
		for _, forma := range media.Formats {
			clockRate := forma.ClockRate()

			switch forma := forma.(type) {
			case *rtspformat.H265: //nolint:dupl
				track := addTrack(&mpegts.CodecH265{})

				var dtsExtractor *h265.DTSExtractor

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						randomAccess := h265.IsRandomAccess(u.Payload.(unit.PayloadH265))

						if dtsExtractor == nil {
							if !randomAccess {
								return nil
							}
							dtsExtractor = &h265.DTSExtractor{}
							dtsExtractor.Initialize()
						}

						dts, err := dtsExtractor.Extract(u.Payload.(unit.PayloadH265), u.PTS)
						if err != nil {
							return err
						}

						return f.write(
							timestampToDuration(dts, clockRate),
							u.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteH265(
									track,
									u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									dts,
									u.Payload.(unit.PayloadH265))
							},
						)
					})

			case *rtspformat.H264: //nolint:dupl
				track := addTrack(&mpegts.CodecH264{})

				var dtsExtractor *h264.DTSExtractor

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						randomAccess := h264.IsRandomAccess(u.Payload.(unit.PayloadH264))

						if dtsExtractor == nil {
							if !randomAccess {
								return nil
							}
							dtsExtractor = &h264.DTSExtractor{}
							dtsExtractor.Initialize()
						}

						dts, err := dtsExtractor.Extract(u.Payload.(unit.PayloadH264), u.PTS)
						if err != nil {
							return err
						}

						return f.write(
							timestampToDuration(dts, clockRate),
							u.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteH264(
									track,
									u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									dts,
									u.Payload.(unit.PayloadH264))
							},
						)
					})

			case *rtspformat.MPEG4Video:
				track := addTrack(&mpegts.CodecMPEG4Video{})

				firstReceived := false
				var lastPTS int64

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if !firstReceived {
							firstReceived = true
						} else if u.PTS < lastPTS {
							return fmt.Errorf("MPEG-4 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = u.PTS

						randomAccess := bytes.Contains(u.Payload.(unit.PayloadMPEG4Video),
							[]byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})

						return f.write(
							timestampToDuration(u.PTS, clockRate),
							u.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteMPEG4Video(
									track,
									u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									u.Payload.(unit.PayloadMPEG4Video))
							},
						)
					})

			case *rtspformat.MPEG1Video:
				track := addTrack(&mpegts.CodecMPEG1Video{})

				firstReceived := false
				var lastPTS int64

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if !firstReceived {
							firstReceived = true
						} else if u.PTS < lastPTS {
							return fmt.Errorf("MPEG-1 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = u.PTS

						randomAccess := bytes.Contains(u.Payload.(unit.PayloadMPEG1Video), []byte{0, 0, 1, 0xB8})

						return f.write(
							timestampToDuration(u.PTS, clockRate),
							u.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteMPEG1Video(
									track,
									u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									u.Payload.(unit.PayloadMPEG1Video))
							},
						)
					})

			case *rtspformat.Opus:
				track := addTrack(&mpegts.CodecOpus{
					ChannelCount: forma.ChannelCount,
				})

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						return f.write(
							timestampToDuration(u.PTS, clockRate),
							u.NTP,
							false,
							true,
							func() error {
								return f.mw.WriteOpus(
									track,
									multiplyAndDivide(u.PTS, 90000, int64(clockRate)),
									u.Payload.(unit.PayloadOpus))
							},
						)
					})

			case *rtspformat.KLV:
				track := addTrack(&mpegts.CodecKLV{
					Synchronous: true,
				})

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						return f.write(
							timestampToDuration(u.PTS, 90000),
							u.NTP,
							false,
							true,
							func() error {
								return f.mw.WriteKLV(track, multiplyAndDivide(u.PTS, 90000, 90000), u.Payload.(unit.PayloadKLV))
							},
						)
					})

			case *rtspformat.MPEG4Audio:
				track := addTrack(&mpegts.CodecMPEG4Audio{
					Config: *forma.Config,
				})

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						return f.write(
							timestampToDuration(u.PTS, clockRate),
							u.NTP,
							false,
							true,
							func() error {
								return f.mw.WriteMPEG4Audio(
									track,
									multiplyAndDivide(u.PTS, 90000, int64(clockRate)),
									u.Payload.(unit.PayloadMPEG4Audio))
							},
						)
					})

			case *rtspformat.MPEG4AudioLATM:
				if !forma.CPresent {
					track := addTrack(&mpegts.CodecMPEG4Audio{
						Config: *forma.StreamMuxConfig.Programs[0].Layers[0].AudioSpecificConfig,
					})

					f.ri.reader.OnData(
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

							return f.write(
								timestampToDuration(u.PTS, clockRate),
								u.NTP,
								false,
								true,
								func() error {
									return f.mw.WriteMPEG4Audio(
										track,
										multiplyAndDivide(u.PTS, 90000, int64(clockRate)),
										[][]byte{ame.Payloads[0][0][0]})
								},
							)
						})
				}

			case *rtspformat.MPEG1Audio:
				track := addTrack(&mpegts.CodecMPEG1Audio{})

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						return f.write(
							timestampToDuration(u.PTS, clockRate),
							u.NTP,
							false,
							true,
							func() error {
								return f.mw.WriteMPEG1Audio(
									track,
									u.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									u.Payload.(unit.PayloadMPEG1Audio))
							},
						)
					})

			case *rtspformat.AC3:
				track := addTrack(&mpegts.CodecAC3{})

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						return f.write(
							timestampToDuration(u.PTS, clockRate),
							u.NTP,
							false,
							true,
							func() error {
								for i, frame := range u.Payload.(unit.PayloadAC3) {
									framePTS := u.PTS + int64(i)*ac3.SamplesPerFrame

									err := f.mw.WriteAC3(
										track,
										multiplyAndDivide(framePTS, 90000, int64(clockRate)),
										frame)
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

	if len(tracks) == 0 {
		f.ri.Log(logger.Warn, "no supported tracks found, skipping recording")
		return false
	}

	setuppedFormats := f.ri.reader.Formats()

	n := 1
	for _, medi := range f.ri.stream.Desc.Medias {
		for _, forma := range medi.Formats {
			if !slices.Contains(setuppedFormats, forma) {
				f.ri.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	f.dw = &dynamicWriter{}
	f.bw = bufio.NewWriterSize(f.dw, mpegtsBufferSize)

	f.mw = &mpegts.Writer{W: f.bw, Tracks: tracks}
	err := f.mw.Initialize()
	if err != nil {
		panic(err)
	}

	f.ri.Log(logger.Info, "recording %s",
		defs.FormatsInfo(setuppedFormats))

	return true
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
			pathFormat2:       f.ri.pathFormat2,
			flush:             f.bw.Flush,
			onSegmentCreate:   f.ri.onSegmentCreate,
			onSegmentComplete: f.ri.onSegmentComplete,
			startDTS:          dts,
			startNTP:          ntp,
			log:               f.ri,
		}
		f.currentSegment.initialize()
		f.dw.setTarget(f.currentSegment)
	case (!f.hasVideo || isVideo) &&
		randomAccess &&
		(dts-f.currentSegment.startDTS) >= f.ri.segmentDuration:
		f.currentSegment.lastDTS = dts
		err := f.currentSegment.close()
		if err != nil {
			return err
		}

		f.currentSegment = &formatMPEGTSSegment{
			pathFormat2:       f.ri.pathFormat2,
			flush:             f.bw.Flush,
			onSegmentCreate:   f.ri.onSegmentCreate,
			onSegmentComplete: f.ri.onSegmentComplete,
			startDTS:          dts,
			startNTP:          ntp,
			log:               f.ri,
		}
		f.currentSegment.initialize()
		f.dw.setTarget(f.currentSegment)

	case (dts - f.currentSegment.lastFlush) >= f.ri.partDuration:
		err := f.bw.Flush()
		if err != nil {
			return err
		}

		f.currentSegment.lastFlush = dts
	}

	f.currentSegment.lastDTS = dts

	return writeCB()
}
