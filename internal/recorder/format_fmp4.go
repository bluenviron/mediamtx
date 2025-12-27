package recorder

import (
	"bytes"
	"fmt"
	"slices"
	"time"

	rtspformat "github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/g711"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/jpeg"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/vp9"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"

	"github.com/bluenviron/mediamtx/internal/codecprocessor"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var av1DefaultSequenceHeader = []byte{
	8, 0, 0, 0, 66, 167, 191, 228, 96, 13, 0, 64,
}

func mpeg1audioChannelCount(cm mpeg1audio.ChannelMode) int {
	switch cm {
	case mpeg1audio.ChannelModeStereo,
		mpeg1audio.ChannelModeJointStereo,
		mpeg1audio.ChannelModeDualChannel:
		return 2

	default:
		return 1
	}
}

func jpegExtractSize(image []byte) (int, int, error) {
	l := len(image)
	if l < 2 || image[0] != 0xFF || image[1] != jpeg.MarkerStartOfImage {
		return 0, 0, fmt.Errorf("invalid header")
	}

	image = image[2:]

	for {
		if len(image) < 2 {
			return 0, 0, fmt.Errorf("not enough bits")
		}

		h0, h1 := image[0], image[1]
		image = image[2:]

		if h0 != 0xFF {
			return 0, 0, fmt.Errorf("invalid image")
		}

		switch h1 {
		case 0xE0, 0xE1, 0xE2, // JFIF
			jpeg.MarkerDefineHuffmanTable,
			jpeg.MarkerComment,
			jpeg.MarkerDefineQuantizationTable,
			jpeg.MarkerDefineRestartInterval:
			mlen := int(image[0])<<8 | int(image[1])
			if len(image) < mlen {
				return 0, 0, fmt.Errorf("not enough bits")
			}
			image = image[mlen:]

		case jpeg.MarkerStartOfFrame1:
			mlen := int(image[0])<<8 | int(image[1])
			if len(image) < mlen {
				return 0, 0, fmt.Errorf("not enough bits")
			}

			var sof jpeg.StartOfFrame1
			err := sof.Unmarshal(image[2:mlen])
			if err != nil {
				return 0, 0, err
			}

			return sof.Width, sof.Height, nil

		case jpeg.MarkerStartOfScan:
			return 0, 0, fmt.Errorf("SOF not found")

		default:
			return 0, 0, fmt.Errorf("unknown marker: 0x%.2x", h1)
		}
	}
}

type formatFMP4Sample struct {
	*fmp4.Sample
	dts int64
	ntp time.Time
}

type formatFMP4 struct {
	ri *recorderInstance

	tracks            []*formatFMP4Track
	hasVideo          bool
	currentSegment    *formatFMP4Segment
	nextSegmentNumber uint64
}

func (f *formatFMP4) initialize() bool {
	nextID := 1

	addTrack := func(format rtspformat.Format, codec mcodecs.Codec) *formatFMP4Track {
		track := &formatFMP4Track{
			f:         f,
			id:        nextID,
			clockRate: uint32(format.ClockRate()),
			codec:     codec,
		}
		track.initialize()

		nextID++
		f.tracks = append(f.tracks, track)
		return track
	}

	for _, media := range f.ri.stream.Desc.Medias {
		for _, forma := range media.Formats {
			clockRate := forma.ClockRate()

			switch forma := forma.(type) {
			case *rtspformat.AV1:
				codec := &mcodecs.AV1{
					SequenceHeader: av1DefaultSequenceHeader,
				}
				track := addTrack(forma, codec)

				firstReceived := false

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						randomAccess := false
						paramsChanged := false

						for _, obu := range u.Payload.(unit.PayloadAV1) {
							typ := av1.OBUType((obu[0] >> 3) & 0b1111)

							if typ == av1.OBUTypeSequenceHeader {
								if !bytes.Equal(codec.SequenceHeader, obu) {
									codec.SequenceHeader = obu
									paramsChanged = true
								}
								randomAccess = true
							}
						}

						if paramsChanged {
							f.updateCodecParams()
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						}

						var sampl fmp4.Sample
						err := sampl.FillAV1(u.Payload.(unit.PayloadAV1))
						if err != nil {
							return err
						}

						return track.write(&formatFMP4Sample{
							Sample: &sampl,
							dts:    u.PTS,
							ntp:    u.NTP,
						})
					})

			case *rtspformat.VP9:
				codec := &mcodecs.VP9{
					Width:             1280,
					Height:            720,
					Profile:           1,
					BitDepth:          8,
					ChromaSubsampling: 1,
					ColorRange:        false,
				}
				track := addTrack(forma, codec)

				firstReceived := false

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						var h vp9.Header
						err := h.Unmarshal(u.Payload.(unit.PayloadVP9))
						if err != nil {
							return err
						}

						randomAccess := false
						paramsChanged := false

						if !h.NonKeyFrame {
							randomAccess = true

							if w := h.Width(); codec.Width != w {
								codec.Width = w
								paramsChanged = true
							}
							if h := h.Width(); codec.Height != h {
								codec.Height = h
								paramsChanged = true
							}
							if codec.Profile != h.Profile {
								codec.Profile = h.Profile
								paramsChanged = true
							}
							if codec.BitDepth != h.ColorConfig.BitDepth {
								codec.BitDepth = h.ColorConfig.BitDepth
								paramsChanged = true
							}
							if c := h.ChromaSubsampling(); codec.ChromaSubsampling != c {
								codec.ChromaSubsampling = c
								paramsChanged = true
							}
							if codec.ColorRange != h.ColorConfig.ColorRange {
								codec.ColorRange = h.ColorConfig.ColorRange
								paramsChanged = true
							}
						}

						if paramsChanged {
							f.updateCodecParams()
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						}

						return track.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								IsNonSyncSample: !randomAccess,
								Payload:         u.Payload.(unit.PayloadVP9),
							},
							dts: u.PTS,
							ntp: u.NTP,
						})
					})

			case *rtspformat.VP8:
				// TODO

			case *rtspformat.H265:
				vps, sps, pps := forma.SafeParams()
				if vps == nil || sps == nil || pps == nil {
					vps = codecprocessor.H265DefaultVPS
					sps = codecprocessor.H265DefaultSPS
					pps = codecprocessor.H265DefaultPPS
				}

				codec := &mcodecs.H265{
					VPS: vps,
					SPS: sps,
					PPS: pps,
				}
				track := addTrack(forma, codec)

				var dtsExtractor *h265.DTSExtractor

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						randomAccess := false
						paramsChanged := false

						for _, nalu := range u.Payload.(unit.PayloadH265) {
							typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

							switch typ {
							case h265.NALUType_VPS_NUT:
								if !bytes.Equal(codec.VPS, nalu) {
									codec.VPS = nalu
									paramsChanged = true
								}

							case h265.NALUType_SPS_NUT:
								if !bytes.Equal(codec.SPS, nalu) {
									codec.SPS = nalu
									paramsChanged = true
								}

							case h265.NALUType_PPS_NUT:
								if !bytes.Equal(codec.PPS, nalu) {
									codec.PPS = nalu
									paramsChanged = true
								}

							case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
								randomAccess = true
							}
						}

						if paramsChanged {
							f.updateCodecParams()
						}

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

						var sampl fmp4.Sample
						err = sampl.FillH265(int32(u.PTS-dts), u.Payload.(unit.PayloadH265))
						if err != nil {
							return err
						}

						return track.write(&formatFMP4Sample{
							Sample: &sampl,
							dts:    dts,
							ntp:    u.NTP,
						})
					})

			case *rtspformat.H264:
				sps, pps := forma.SafeParams()
				if sps == nil || pps == nil {
					sps = codecprocessor.H264DefaultSPS
					pps = codecprocessor.H264DefaultPPS
				}

				codec := &mcodecs.H264{
					SPS: sps,
					PPS: pps,
				}
				track := addTrack(forma, codec)

				var dtsExtractor *h264.DTSExtractor

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						randomAccess := false
						paramsChanged := false

						for _, nalu := range u.Payload.(unit.PayloadH264) {
							typ := h264.NALUType(nalu[0] & 0x1F)
							switch typ {
							case h264.NALUTypeSPS:
								if !bytes.Equal(codec.SPS, nalu) {
									codec.SPS = nalu
									paramsChanged = true
								}

							case h264.NALUTypePPS:
								if !bytes.Equal(codec.PPS, nalu) {
									codec.PPS = nalu
									paramsChanged = true
								}

							case h264.NALUTypeIDR:
								randomAccess = true
							}
						}

						if paramsChanged {
							f.updateCodecParams()
						}

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

						var sampl fmp4.Sample
						err = sampl.FillH264(int32(u.PTS-dts), u.Payload.(unit.PayloadH264))
						if err != nil {
							return err
						}

						return track.write(&formatFMP4Sample{
							Sample: &sampl,
							dts:    dts,
							ntp:    u.NTP,
						})
					})

			case *rtspformat.MPEG4Video:
				config := forma.SafeParams()

				if config == nil {
					config = codecprocessor.MPEG4VideoDefaultConfig
				}

				codec := &mcodecs.MPEG4Video{
					Config: config,
				}
				track := addTrack(forma, codec)

				firstReceived := false
				var lastPTS int64

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						randomAccess := bytes.Contains(u.Payload.(unit.PayloadMPEG4Video),
							[]byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})

						if bytes.HasPrefix(u.Payload.(unit.PayloadMPEG4Video),
							[]byte{0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode)}) {
							end := bytes.Index(u.Payload.(unit.PayloadMPEG4Video)[4:],
								[]byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})
							if end >= 0 {
								config2 := u.Payload.(unit.PayloadMPEG4Video)[:end+4]

								if !bytes.Equal(codec.Config, config2) {
									codec.Config = config2
									f.updateCodecParams()
								}
							}
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						} else if u.PTS < lastPTS {
							return fmt.Errorf("MPEG-4 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = u.PTS

						return track.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload:         u.Payload.(unit.PayloadMPEG4Video),
								IsNonSyncSample: !randomAccess,
							},
							dts: u.PTS,
							ntp: u.NTP,
						})
					})

			case *rtspformat.MPEG1Video:
				codec := &mcodecs.MPEG1Video{
					Config: codecprocessor.MPEG1VideoDefaultConfig,
				}
				track := addTrack(forma, codec)

				firstReceived := false
				var lastPTS int64

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						randomAccess := bytes.Contains(u.Payload.(unit.PayloadMPEG1Video), []byte{0, 0, 1, 0xB8})

						if bytes.HasPrefix(u.Payload.(unit.PayloadMPEG1Video), []byte{0, 0, 1, 0xB3}) {
							end := bytes.Index(u.Payload.(unit.PayloadMPEG1Video)[4:], []byte{0, 0, 1, 0xB8})
							if end >= 0 {
								config := u.Payload.(unit.PayloadMPEG1Video)[:end+4]

								if !bytes.Equal(codec.Config, config) {
									codec.Config = config
									f.updateCodecParams()
								}
							}
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						} else if u.PTS < lastPTS {
							return fmt.Errorf("MPEG-1 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = u.PTS

						return track.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload:         u.Payload.(unit.PayloadMPEG1Video),
								IsNonSyncSample: !randomAccess,
							},
							dts: u.PTS,
							ntp: u.NTP,
						})
					})

			case *rtspformat.MJPEG:
				codec := &mcodecs.MJPEG{
					Width:  800,
					Height: 600,
				}
				track := addTrack(forma, codec)

				parsed := false

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						if !parsed {
							parsed = true
							width, height, err := jpegExtractSize(u.Payload.(unit.PayloadMJPEG))
							if err != nil {
								return err
							}
							codec.Width = width
							codec.Height = height
							f.updateCodecParams()
						}

						return track.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload: u.Payload.(unit.PayloadMJPEG),
							},
							dts: u.PTS,
							ntp: u.NTP,
						})
					})

			case *rtspformat.Opus:
				codec := &mcodecs.Opus{
					ChannelCount: forma.ChannelCount,
				}
				track := addTrack(forma, codec)

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						pts := u.PTS

						for _, packet := range u.Payload.(unit.PayloadOpus) {
							err := track.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload: packet,
								},
								dts: pts,
								ntp: u.NTP.Add(timestampToDuration(pts-u.PTS, clockRate)),
							})
							if err != nil {
								return err
							}

							pts += opus.PacketDuration2(packet)
						}

						return nil
					})

			case *rtspformat.MPEG4Audio:
				codec := &mcodecs.MPEG4Audio{
					Config: *forma.Config,
				}
				track := addTrack(forma, codec)

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						for i, au := range u.Payload.(unit.PayloadMPEG4Audio) {
							pts := u.PTS + int64(i)*mpeg4audio.SamplesPerAccessUnit

							err := track.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload: au,
								},
								dts: pts,
								ntp: u.NTP.Add(timestampToDuration(pts-u.PTS, clockRate)),
							})
							if err != nil {
								return err
							}
						}

						return nil
					})

			case *rtspformat.MPEG4AudioLATM:
				if !forma.CPresent {
					codec := &mcodecs.MPEG4Audio{
						Config: *forma.StreamMuxConfig.Programs[0].Layers[0].AudioSpecificConfig,
					}
					track := addTrack(forma, codec)

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

							return track.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload: ame.Payloads[0][0][0],
								},
								dts: u.PTS,
								ntp: u.NTP,
							})
						})
				}

			case *rtspformat.MPEG1Audio:
				codec := &mcodecs.MPEG1Audio{
					SampleRate:   32000,
					ChannelCount: 2,
				}
				track := addTrack(forma, codec)

				parsed := false

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						var dt time.Duration

						for _, frame := range u.Payload.(unit.PayloadMPEG1Audio) {
							var h mpeg1audio.FrameHeader
							err := h.Unmarshal(frame)
							if err != nil {
								return err
							}

							if !parsed {
								parsed = true
								codec.SampleRate = h.SampleRate
								codec.ChannelCount = mpeg1audioChannelCount(h.ChannelMode)
								f.updateCodecParams()
							}

							err = track.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload: frame,
								},
								dts: u.PTS + u.PTS,
								ntp: u.NTP,
							})
							if err != nil {
								return err
							}

							dt += time.Duration(h.SampleCount()) *
								time.Second / time.Duration(h.SampleRate)
						}

						return nil
					})

			case *rtspformat.AC3:
				codec := &mcodecs.AC3{
					SampleRate:   forma.SampleRate,
					ChannelCount: forma.ChannelCount,
					Fscod:        0,
					Bsid:         8,
					Bsmod:        0,
					Acmod:        7,
					LfeOn:        true,
					BitRateCode:  7,
				}
				track := addTrack(forma, codec)

				parsed := false

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						for i, frame := range u.Payload.(unit.PayloadAC3) {
							var syncInfo ac3.SyncInfo
							err := syncInfo.Unmarshal(frame)
							if err != nil {
								return fmt.Errorf("invalid AC-3 frame: %w", err)
							}

							var bsi ac3.BSI
							err = bsi.Unmarshal(frame[5:])
							if err != nil {
								return fmt.Errorf("invalid AC-3 frame: %w", err)
							}

							if !parsed {
								parsed = true
								codec.SampleRate = syncInfo.SampleRate()
								codec.ChannelCount = bsi.ChannelCount()
								codec.Fscod = syncInfo.Fscod
								codec.Bsid = bsi.Bsid
								codec.Bsmod = bsi.Bsmod
								codec.Acmod = bsi.Acmod
								codec.LfeOn = bsi.LfeOn
								codec.BitRateCode = syncInfo.Frmsizecod >> 1
								f.updateCodecParams()
							}

							pts := u.PTS + int64(i)*ac3.SamplesPerFrame

							err = track.write(&formatFMP4Sample{
								Sample: &fmp4.Sample{
									Payload: frame,
								},
								dts: pts,
								ntp: u.NTP.Add(timestampToDuration(pts-u.PTS, clockRate)),
							})
							if err != nil {
								return err
							}
						}

						return nil
					})

			case *rtspformat.G722:
				// TODO

			case *rtspformat.G711:
				codec := &mcodecs.LPCM{
					LittleEndian: false,
					BitDepth:     16,
					SampleRate:   forma.SampleRate,
					ChannelCount: forma.ChannelCount,
				}
				track := addTrack(forma, codec)

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						var lpcm []byte
						if forma.MULaw {
							var mu g711.Mulaw
							mu.Unmarshal(u.Payload.(unit.PayloadG711))
							lpcm = mu
						} else {
							var al g711.Alaw
							al.Unmarshal(u.Payload.(unit.PayloadG711))
							lpcm = al
						}

						return track.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload: lpcm,
							},
							dts: u.PTS,
							ntp: u.NTP,
						})
					})

			case *rtspformat.LPCM:
				codec := &mcodecs.LPCM{
					LittleEndian: false,
					BitDepth:     forma.BitDepth,
					SampleRate:   forma.SampleRate,
					ChannelCount: forma.ChannelCount,
				}
				track := addTrack(forma, codec)

				f.ri.reader.OnData(
					media,
					forma,
					func(u *unit.Unit) error {
						if u.NilPayload() {
							return nil
						}

						return track.write(&formatFMP4Sample{
							Sample: &fmp4.Sample{
								Payload: u.Payload.(unit.PayloadLPCM),
							},
							dts: u.PTS,
							ntp: u.NTP,
						})
					})
			}
		}
	}

	if len(f.tracks) == 0 {
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

	f.ri.Log(logger.Info, "recording %s",
		defs.FormatsInfo(setuppedFormats))

	return true
}

func (f *formatFMP4) updateCodecParams() {
	f.ri.Log(logger.Debug, "codec parameters have changed")
}

func (f *formatFMP4) close() {
	if f.currentSegment != nil {
		f.currentSegment.close() //nolint:errcheck
	}
}
