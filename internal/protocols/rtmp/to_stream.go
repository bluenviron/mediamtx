package rtmp

import (
	"errors"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecsTo = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"AV1, VP9, H265, H264, MPEG-4 Audio, MPEG-1/2 Audio, G711, LPCM")

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func durationToTimestamp(d time.Duration, clockRate int) int64 {
	return multiplyAndDivide(int64(d), int64(clockRate), int64(time.Second))
}

func fourCCToString(c message.FourCC) string {
	return string([]byte{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)})
}

// ToStream maps a RTMP stream to a MediaMTX stream.
func ToStream(
	r *gortmplib.Reader,
	subStream **stream.SubStream,
) ([]*description.Media, error) {
	var medias []*description.Media

	for _, track := range r.Tracks() {
		switch codec := track.Codec.(type) {
		case *codecs.AV1:
			forma := &format.AV1{
				PayloadTyp: 96,
			}
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataAV1(track, func(pts time.Duration, tu [][]byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadAV1(tu),
				})
			})

		case *codecs.VP9:
			forma := &format.VP9{
				PayloadTyp: 96,
			}
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataVP9(track, func(pts time.Duration, frame []byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadVP9(frame),
				})
			})

		case *codecs.H265:
			forma := &format.H265{
				PayloadTyp: 96,
				VPS:        codec.VPS,
				SPS:        codec.SPS,
				PPS:        codec.PPS,
			}
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataH265(track, func(pts time.Duration, _ time.Duration, au [][]byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadH265(au),
				})
			})

		case *codecs.H264:
			forma := &format.H264{
				PayloadTyp:        96,
				SPS:               codec.SPS,
				PPS:               codec.PPS,
				PacketizationMode: 1,
			}
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataH264(track, func(pts time.Duration, _ time.Duration, au [][]byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadH264(au),
				})
			})

		case *codecs.Opus:
			forma := &format.Opus{
				PayloadTyp:   96,
				ChannelCount: codec.ChannelCount,
			}
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataOpus(track, func(pts time.Duration, packet []byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadOpus{packet},
				})
			})

		case *codecs.MPEG4Audio:
			forma := &format.MPEG4Audio{
				PayloadTyp:       96,
				Config:           codec.Config,
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataMPEG4Audio(track, func(pts time.Duration, au []byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadMPEG4Audio{au},
				})
			})

		case *codecs.MPEG1Audio:
			forma := &format.MPEG1Audio{}
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataMPEG1Audio(track, func(pts time.Duration, frame []byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadMPEG1Audio{frame},
				})
			})

		case *codecs.AC3:
			forma := &format.AC3{
				PayloadTyp:   96,
				SampleRate:   codec.SampleRate,
				ChannelCount: codec.ChannelCount,
			}
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataAC3(track, func(pts time.Duration, frame []byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadAC3{frame},
				})
			})

		case *codecs.G711:
			forma := &format.G711{
				PayloadTyp: func() uint8 {
					switch {
					case codec.ChannelCount == 1 && codec.MULaw:
						return 0
					case codec.ChannelCount == 1 && !codec.MULaw:
						return 8
					default:
						return 96
					}
				}(),
				MULaw:        codec.MULaw,
				SampleRate:   8000,
				ChannelCount: codec.ChannelCount,
			}
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataG711(track, func(pts time.Duration, samples []byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadG711(samples),
				})
			})

		case *codecs.LPCM:
			forma := &format.LPCM{
				PayloadTyp:   96,
				BitDepth:     codec.BitDepth,
				SampleRate:   codec.SampleRate,
				ChannelCount: codec.ChannelCount,
			}
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, medi)

			r.OnDataLPCM(track, func(pts time.Duration, samples []byte) {
				(*subStream).WriteUnit(medi, forma, &unit.Unit{
					PTS:     durationToTimestamp(pts, forma.ClockRate()),
					Payload: unit.PayloadLPCM(samples),
				})
			})

		default:
			panic("should not happen")
		}
	}

	if len(medias) == 0 {
		return nil, errNoSupportedCodecsTo
	}

	return medias, nil
}
