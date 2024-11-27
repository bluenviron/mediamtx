package hls

import (
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

// ToStream maps a HLS stream to a MediaMTX stream.
func ToStream(
	c *gohlslib.Client,
	tracks []*gohlslib.Track,
	stream **stream.Stream,
) ([]*description.Media, error) {
	var medias []*description.Media //nolint:prealloc

	for _, track := range tracks {
		var medi *description.Media
		clockRate := track.ClockRate

		switch tcodec := track.Codec.(type) {
		case *codecs.AV1:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.AV1{
					PayloadTyp: 96,
				}},
			}

			c.OnDataAV1(track, func(pts int64, tu [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.AV1{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					TU: tu,
				})
			})

		case *codecs.VP9:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.VP9{
					PayloadTyp: 96,
				}},
			}

			c.OnDataVP9(track, func(pts int64, frame []byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.VP9{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					Frame: frame,
				})
			})

		case *codecs.H265:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
					VPS:        tcodec.VPS,
					SPS:        tcodec.SPS,
					PPS:        tcodec.PPS,
				}},
			}

			c.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					AU: au,
				})
			})

		case *codecs.H264:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
					SPS:               tcodec.SPS,
					PPS:               tcodec.PPS,
				}},
			}

			c.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					AU: au,
				})
			})

		case *codecs.Opus:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: tcodec.ChannelCount,
				}},
			}

			c.OnDataOpus(track, func(pts int64, packets [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.Opus{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), int64(clockRate)),
					},
					Packets: packets,
				})
			})

		case *codecs.MPEG4Audio:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config:           &tcodec.Config,
				}},
			}

			c.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), int64(clockRate)),
					},
					AUs: aus,
				})
			})

		default:
			panic("should not happen")
		}

		medias = append(medias, medi)
	}

	if len(medias) == 0 {
		return nil, ErrNoSupportedCodecs
	}

	return medias, nil
}
