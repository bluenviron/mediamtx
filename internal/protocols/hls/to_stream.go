package hls

import (
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// ToStream maps a HLS stream to a MediaMTX stream.
func ToStream(
	c *gohlslib.Client,
	tracks []*gohlslib.Track,
	stream **stream.Stream,
) ([]*description.Media, error) {
	var medias []*description.Media //nolint:prealloc

	for _, track := range tracks {
		var medi *description.Media

		switch tcodec := track.Codec.(type) {
		case *codecs.AV1:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.AV1{
					PayloadTyp: 96,
				}},
			}

			c.OnDataAV1(track, func(pts time.Duration, tu [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.AV1{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
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

			c.OnDataVP9(track, func(pts time.Duration, frame []byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.VP9{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Frame: frame,
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

			c.OnDataH26x(track, func(pts time.Duration, _ time.Duration, au [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AU: au,
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

			c.OnDataH26x(track, func(pts time.Duration, _ time.Duration, au [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AU: au,
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

			c.OnDataMPEG4Audio(track, func(pts time.Duration, aus [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AUs: aus,
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

			c.OnDataOpus(track, func(pts time.Duration, packets [][]byte) {
				(*stream).WriteUnit(medi, medi.Formats[0], &unit.Opus{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Packets: packets,
				})
			})

		default:
			return nil, fmt.Errorf("unsupported track: %T", track.Codec)
		}

		medias = append(medias, medi)
	}

	return medias, nil
}
