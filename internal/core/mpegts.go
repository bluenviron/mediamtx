package core

import (
	"bufio"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errMPEGTSNoTracks = errors.New("no supported tracks found (supported are H265, H264," +
	" MPEG-4 Video, MPEG-1/2 Video, Opus, MPEG-4 Audio, MPEG-1 Audio, AC-3")

func durationGoToMPEGTS(v time.Duration) int64 {
	return int64(v.Seconds() * 90000)
}

func mpegtsSetupRead(r *mpegts.Reader, stream **stream.Stream) ([]*description.Media, error) {
	var medias []*description.Media //nolint:prealloc

	var td *mpegts.TimeDecoder
	decodeTime := func(t int64) time.Duration {
		if td == nil {
			td = mpegts.NewTimeDecoder(t)
		}
		return td.Decode(t)
	}

	for _, track := range r.Tracks() { //nolint:dupl
		var medi *description.Media

		switch codec := track.Codec.(type) {
		case *mpegts.CodecH265:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecH264:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Video:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG4Video{
					PayloadTyp: 96,
				}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Video{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					Frame: frame,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Video:
			medi = &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG1Video{}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG1Video{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					Frame: frame,
				})
				return nil
			})

		case *mpegts.CodecOpus:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp: 96,
					IsStereo:   (codec.ChannelCount == 2),
				}},
			}

			r.OnDataOpus(track, func(pts int64, packets [][]byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.Opus{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					Packets: packets,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Audio:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config:           &codec.Config,
				}},
			}

			r.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					AUs: aus,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Audio:
			medi = &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG1Audio{}},
			}

			r.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					Frames: frames,
				})
				return nil
			})

		case *mpegts.CodecAC3:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.AC3{
					PayloadTyp:   96,
					SampleRate:   codec.SampleRate,
					ChannelCount: codec.ChannelCount,
				}},
			}

			r.OnDataAC3(track, func(pts int64, frame []byte) error {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.AC3{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					Frames: [][]byte{frame},
				})
				return nil
			})

		default:
			continue
		}

		medias = append(medias, medi)
	}

	if len(medias) == 0 {
		return nil, errMPEGTSNoTracks
	}

	return medias, nil
}

func mpegtsSetupWrite(
	stream *stream.Stream,
	writer *asyncwriter.Writer,
	bw *bufio.Writer,
	sconn srt.Conn,
	writeTimeout time.Duration,
) error {
	var w *mpegts.Writer
	var tracks []*mpegts.Track

	addTrack := func(codec mpegts.Codec) *mpegts.Track {
		track := &mpegts.Track{
			Codec: codec,
		}
		tracks = append(tracks, track)
		return track
	}

	for _, medi := range stream.Desc().Medias {
		for _, forma := range medi.Formats {
			switch forma := forma.(type) {
			case *format.H265: //nolint:dupl
				track := addTrack(&mpegts.CodecH265{})

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
					err = (*w).WriteH26x(track, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), randomAccess, tunit.AU)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.H264: //nolint:dupl
				track := addTrack(&mpegts.CodecH264{})

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
					err = (*w).WriteH26x(track, durationGoToMPEGTS(tunit.PTS), durationGoToMPEGTS(dts), idrPresent, tunit.AU)
					if err != nil {
						return err
					}
					return bw.Flush()
				})

			case *format.MPEG4Video:
				track := addTrack(&mpegts.CodecMPEG4Video{})

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
				track := addTrack(&mpegts.CodecMPEG1Video{})

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
				track := addTrack(&mpegts.CodecOpus{
					ChannelCount: func() int {
						if forma.IsStereo {
							return 2
						}
						return 1
					}(),
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
				track := addTrack(&mpegts.CodecMPEG4Audio{
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
				track := addTrack(&mpegts.CodecMPEG1Audio{})

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
				track := addTrack(&mpegts.CodecAC3{})

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
		return errMPEGTSNoTracks
	}

	w = mpegts.NewWriter(bw, tracks)

	return nil
}
