package record

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/opus"
	"github.com/bluenviron/mediacommon/pkg/codecs/vp9"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func durationGoToMp4(v time.Duration, timeScale uint32) uint64 {
	timeScale64 := uint64(timeScale)
	secs := v / time.Second
	dec := v % time.Second
	return uint64(secs)*timeScale64 + uint64(dec)*timeScale64/uint64(time.Second)
}

type sample struct {
	*fmp4.PartSample
	dts time.Duration
}

// Agent saves streams on disk.
type Agent struct {
	path            string
	partDuration    time.Duration
	segmentDuration time.Duration
	stream          *stream.Stream
	parent          logger.Writer

	ctx            context.Context
	ctxCancel      func()
	writer         *asyncwriter.Writer
	tracks         []*track
	hasVideo       bool
	currentSegment *segment

	done chan struct{}
}

// NewAgent allocates a nAgent.
func NewAgent(
	writeQueueSize int,
	path string,
	partDuration time.Duration,
	segmentDuration time.Duration,
	pathName string,
	stream *stream.Stream,
	parent logger.Writer,
) *Agent {
	path, _ = filepath.Abs(path)
	path = strings.ReplaceAll(path, "%path", pathName)
	path += ".mp4"

	ctx, ctxCancel := context.WithCancel(context.Background())

	r := &Agent{
		path:            path,
		partDuration:    partDuration,
		segmentDuration: segmentDuration,
		stream:          stream,
		parent:          parent,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		done:            make(chan struct{}),
	}

	r.writer = asyncwriter.New(writeQueueSize, r)

	nextID := 1

	addTrack := func(initTrack *fmp4.InitTrack, isVideo bool) *track {
		initTrack.ID = nextID
		nextID++

		track := newTrack(r, initTrack, isVideo)
		r.tracks = append(r.tracks, track)

		return track
	}

	for _, media := range stream.Desc().Medias {
		for _, forma := range media.Formats {
			switch forma := forma.(type) {
			case *format.AV1:
				codec := &fmp4.CodecAV1{
					SequenceHeader: []byte{
						8, 0, 0, 0, 66, 167, 191, 228, 96, 13, 0, 64,
					},
				}

				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec:     codec,
				}, true)

				firstReceived := false

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.AV1)
					if tunit.TU == nil {
						return nil
					}

					randomAccess := false

					for _, obu := range tunit.TU {
						var h av1.OBUHeader
						err := h.Unmarshal(obu)
						if err != nil {
							return err
						}

						if h.Type == av1.OBUTypeSequenceHeader {
							codec.SequenceHeader = obu
							randomAccess = true
						}
					}

					if !firstReceived {
						if !randomAccess {
							return nil
						}
						firstReceived = true
					}

					sampl, err := fmp4.NewPartSampleAV1(
						randomAccess,
						tunit.TU)
					if err != nil {
						return err
					}

					return track.record(&sample{
						PartSample: sampl,
						dts:        tunit.PTS,
					})
				})

			case *format.VP9:
				codec := &fmp4.CodecVP9{
					Width:             1280,
					Height:            720,
					Profile:           1,
					BitDepth:          8,
					ChromaSubsampling: 1,
					ColorRange:        false,
				}

				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec:     codec,
				}, true)

				firstReceived := false

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.VP9)
					if tunit.Frame == nil {
						return nil
					}

					var h vp9.Header
					err := h.Unmarshal(tunit.Frame)
					if err != nil {
						return err
					}

					randomAccess := false

					if h.FrameType == vp9.FrameTypeKeyFrame {
						randomAccess = true
						codec.Width = h.Width()
						codec.Height = h.Height()
						codec.Profile = h.Profile
						codec.BitDepth = h.ColorConfig.BitDepth
						codec.ChromaSubsampling = h.ChromaSubsampling()
						codec.ColorRange = h.ColorConfig.ColorRange
					}

					if !firstReceived {
						if !randomAccess {
							return nil
						}
						firstReceived = true
					}

					return track.record(&sample{
						PartSample: &fmp4.PartSample{
							IsNonSyncSample: !randomAccess,
							Payload:         tunit.Frame,
						},
						dts: tunit.PTS,
					})
				})

			case *format.VP8:
				// TODO

			case *format.H265:
				vps, sps, pps := forma.SafeParams()

				if vps == nil || sps == nil || pps == nil {
					vps = []byte{
						0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x02, 0x20,
						0x00, 0x00, 0x03, 0x00, 0xb0, 0x00, 0x00, 0x03,
						0x00, 0x00, 0x03, 0x00, 0x7b, 0x18, 0xb0, 0x24,
					}

					sps = []byte{
						0x42, 0x01, 0x01, 0x02, 0x20, 0x00, 0x00, 0x03,
						0x00, 0xb0, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
						0x00, 0x7b, 0xa0, 0x07, 0x82, 0x00, 0x88, 0x7d,
						0xb6, 0x71, 0x8b, 0x92, 0x44, 0x80, 0x53, 0x88,
						0x88, 0x92, 0xcf, 0x24, 0xa6, 0x92, 0x72, 0xc9,
						0x12, 0x49, 0x22, 0xdc, 0x91, 0xaa, 0x48, 0xfc,
						0xa2, 0x23, 0xff, 0x00, 0x01, 0x00, 0x01, 0x6a,
						0x02, 0x02, 0x02, 0x01,
					}

					pps = []byte{
						0x44, 0x01, 0xc0, 0x25, 0x2f, 0x05, 0x32, 0x40,
					}
				}

				codec := &fmp4.CodecH265{
					VPS: vps,
					SPS: sps,
					PPS: pps,
				}

				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec:     codec,
				}, true)

				var dtsExtractor *h265.DTSExtractor

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.H265)
					if tunit.AU == nil {
						return nil
					}

					for _, nalu := range tunit.AU {
						typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

						switch typ {
						case h265.NALUType_VPS_NUT:
							codec.VPS = nalu

						case h265.NALUType_SPS_NUT:
							codec.SPS = nalu

						case h265.NALUType_PPS_NUT:
							codec.PPS = nalu
						}
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

					sampl, err := fmp4.NewPartSampleH26x(
						int32(durationGoToMp4(tunit.PTS-dts, 90000)),
						randomAccess,
						tunit.AU)
					if err != nil {
						return err
					}

					return track.record(&sample{
						PartSample: sampl,
						dts:        dts,
					})
				})

			case *format.H264:
				sps, pps := forma.SafeParams()

				if sps == nil || pps == nil {
					sps = []byte{
						0x67, 0x42, 0xc0, 0x1f, 0xd9, 0x00, 0xf0, 0x11,
						0x7e, 0xf0, 0x11, 0x00, 0x00, 0x03, 0x00, 0x01,
						0x00, 0x00, 0x03, 0x00, 0x30, 0x8f, 0x18, 0x32,
						0x48,
					}

					pps = []byte{
						0x68, 0xcb, 0x8c, 0xb2,
					}
				}

				codec := &fmp4.CodecH264{
					SPS: sps,
					PPS: pps,
				}

				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec:     codec,
				}, true)

				var dtsExtractor *h264.DTSExtractor

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.H264)
					if tunit.AU == nil {
						return nil
					}

					randomAccess := false

					for _, nalu := range tunit.AU {
						typ := h264.NALUType(nalu[0] & 0x1F)
						switch typ {
						case h264.NALUTypeSPS:
							codec.SPS = nalu

						case h264.NALUTypePPS:
							codec.PPS = nalu

						case h264.NALUTypeIDR:
							randomAccess = true
						}
					}

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

					sampl, err := fmp4.NewPartSampleH26x(
						int32(durationGoToMp4(tunit.PTS-dts, 90000)),
						randomAccess,
						tunit.AU)
					if err != nil {
						return err
					}

					return track.record(&sample{
						PartSample: sampl,
						dts:        dts,
					})
				})

			case *format.MPEG4Video:
				// TODO

			case *format.MPEG1Video:
				// TODO

			case *format.MJPEG:
				// TODO

			case *format.Opus:
				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec: &fmp4.CodecOpus{
						ChannelCount: func() int {
							if forma.IsStereo {
								return 2
							}
							return 1
						}(),
					},
				}, false)

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.Opus)
					if tunit.Packets == nil {
						return nil
					}

					pts := tunit.PTS

					for _, packet := range tunit.Packets {
						err := track.record(&sample{
							PartSample: &fmp4.PartSample{
								Payload: packet,
							},
							dts: pts,
						})
						if err != nil {
							return err
						}

						pts += opus.PacketDuration(packet)
					}

					return nil
				})

			case *format.MPEG4AudioGeneric:
				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec: &fmp4.CodecMPEG4Audio{
						Config: *forma.Config,
					},
				}, false)

				sampleRate := time.Duration(forma.Config.SampleRate)

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG4AudioGeneric)
					if tunit.AUs == nil {
						return nil
					}

					for i, au := range tunit.AUs {
						auPTS := tunit.PTS + time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
							time.Second/sampleRate

						err := track.record(&sample{
							PartSample: &fmp4.PartSample{
								Payload: au,
							},
							dts: auPTS,
						})
						if err != nil {
							return err
						}
					}

					return nil
				})

			case *format.MPEG4AudioLATM:
				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec: &fmp4.CodecMPEG4Audio{
						Config: *forma.Config.Programs[0].Layers[0].AudioSpecificConfig,
					},
				}, false)

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG4AudioLATM)
					if tunit.AU == nil {
						return nil
					}

					return track.record(&sample{
						PartSample: &fmp4.PartSample{
							Payload: tunit.AU,
						},
						dts: tunit.PTS,
					})
				})

			case *format.MPEG1Audio:
				codec := &fmp4.CodecMPEG1Audio{
					SampleRate:   32000,
					ChannelCount: 2,
				}

				track := addTrack(&fmp4.InitTrack{
					TimeScale: 90000,
					Codec:     codec,
				}, false)

				stream.AddReader(r.writer, media, forma, func(u unit.Unit) error {
					tunit := u.(*unit.MPEG1Audio)
					if tunit.Frames == nil {
						return nil
					}

					pts := tunit.PTS

					for _, frame := range tunit.Frames {
						var h mpeg1audio.FrameHeader
						err := h.Unmarshal(frame)
						if err != nil {
							return err
						}

						codec.SampleRate = h.SampleRate

						switch h.ChannelMode {
						case mpeg1audio.ChannelModeStereo,
							mpeg1audio.ChannelModeJointStereo,
							mpeg1audio.ChannelModeDualChannel:
							codec.ChannelCount = 2

						default:
							codec.ChannelCount = 1
						}

						err = track.record(&sample{
							PartSample: &fmp4.PartSample{
								Payload: frame,
							},
							dts: pts,
						})
						if err != nil {
							return err
						}

						pts += time.Duration(h.SampleCount()) *
							time.Second / time.Duration(h.SampleRate)
					}

					return nil
				})

			case *format.G722:
				// TODO

			case *format.G711:
				// TODO

			case *format.LPCM:
				// TODO
			}
		}
	}

	r.Log(logger.Info, "recording %d %s",
		len(r.tracks),
		func() string {
			if len(r.tracks) == 1 {
				return "track"
			}
			return "tracks"
		}())

	go r.run()

	return r
}

// Close closes the Agent.
func (r *Agent) Close() {
	r.ctxCancel()
	<-r.done
}

// Log is the main logging function.
func (r *Agent) Log(level logger.Level, format string, args ...interface{}) {
	r.parent.Log(level, "[record] "+format, args...)
}

func (r *Agent) run() {
	close(r.done)

	r.writer.Start()

	select {
	case err := <-r.writer.Error():
		r.Log(logger.Error, err.Error())
		r.stream.RemoveReader(r.writer)

	case <-r.ctx.Done():
		r.stream.RemoveReader(r.writer)
		r.writer.Stop()
	}

	if r.currentSegment != nil {
		r.currentSegment.close() //nolint:errcheck
	}
}
