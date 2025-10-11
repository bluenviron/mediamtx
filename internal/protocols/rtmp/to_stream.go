package rtmp

import (
	"errors"
	"time"

	"github.com/bluenviron/gortmplib"
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
	strm **stream.Stream,
) ([]*description.Media, error) {
	var medias []*description.Media

	for _, track := range r.Tracks() {
		ctrack := track

		switch ttrack := track.(type) {
		case *format.AV1:
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataAV1(ttrack, func(pts time.Duration, tu [][]byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadAV1(tu),
				})
			})

		case *format.VP9:
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataVP9(ttrack, func(pts time.Duration, frame []byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadVP9(frame),
				})
			})

		case *format.H265:
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataH265(ttrack, func(pts time.Duration, _ time.Duration, au [][]byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadH265(au),
				})
			})

		case *format.H264:
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataH264(ttrack, func(pts time.Duration, _ time.Duration, au [][]byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadH264(au),
				})
			})

		case *format.Opus:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataOpus(ttrack, func(pts time.Duration, packet []byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadOpus{packet},
				})
			})

		case *format.MPEG4Audio:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataMPEG4Audio(ttrack, func(pts time.Duration, au []byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadMPEG4Audio{au},
				})
			})

		case *format.MPEG1Audio:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataMPEG1Audio(ttrack, func(pts time.Duration, frame []byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadMPEG1Audio{frame},
				})
			})

		case *format.AC3:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataAC3(ttrack, func(pts time.Duration, frame []byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadAC3{frame},
				})
			})

		case *format.G711:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataG711(ttrack, func(pts time.Duration, samples []byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
					Payload: unit.PayloadG711(samples),
				})
			})

		case *format.LPCM:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataLPCM(ttrack, func(pts time.Duration, samples []byte) {
				(*strm).WriteUnit(medi, ctrack, &unit.Unit{
					PTS:     durationToTimestamp(pts, ctrack.ClockRate()),
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
