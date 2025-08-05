package rtmp

import (
	"errors"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
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

// ToStream maps a RTMP stream to a MediaMTX stream.
func ToStream(r *Reader, stream **stream.Stream) ([]*description.Media, error) {
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
				(*stream).WriteUnit(medi, ctrack, &unit.AV1{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					TU: tu,
				})
			})

		case *format.VP9:
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataVP9(ttrack, func(pts time.Duration, frame []byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.VP9{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					Frame: frame,
				})
			})

		case *format.H265:
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataH265(ttrack, func(pts time.Duration, au [][]byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					AU: au,
				})
			})

		case *format.H264:
			medi := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataH264(ttrack, func(pts time.Duration, au [][]byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					AU: au,
				})
			})

		case *format.Opus:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataOpus(ttrack, func(pts time.Duration, packet []byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.Opus{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					Packets: [][]byte{packet},
				})
			})

		case *format.MPEG4Audio:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataMPEG4Audio(ttrack, func(pts time.Duration, au []byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					AUs: [][]byte{au},
				})
			})

		case *format.MPEG1Audio:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataMPEG1Audio(ttrack, func(pts time.Duration, frame []byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					Frames: [][]byte{frame},
				})
			})

		case *format.AC3:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataAC3(ttrack, func(pts time.Duration, frame []byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.AC3{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					Frames: [][]byte{frame},
				})
			})

		case *format.G711:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataG711(ttrack, func(pts time.Duration, samples []byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.G711{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					Samples: samples,
				})
			})

		case *format.LPCM:
			medi := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{ctrack},
			}
			medias = append(medias, medi)

			r.OnDataLPCM(ttrack, func(pts time.Duration, samples []byte) {
				(*stream).WriteUnit(medi, ctrack, &unit.LPCM{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: durationToTimestamp(pts, ctrack.ClockRate()),
					},
					Samples: samples,
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
