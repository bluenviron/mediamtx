package moq

import (
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/catalog"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/property"
	"github.com/bluenviron/mediamtx/internal/protocols/moq/subgroup"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func findTimestamp(props []property.Property) (int64, bool) {
	for _, pr := range props {
		if ts, ok := pr.(*property.Timestamp); ok {
			return int64(*ts), true
		}
	}
	return 0, false
}

// ToStream maps a Media-over-QUIC stream to a MediaMTX stream.
func ToStream(
	cat *catalog.Catalog,
	subStream **stream.SubStream,
) (
	[]*description.Media,
	map[uint64]func(sg *subgroup.SubGroup) error,
	error,
) {
	var medias []*description.Media
	writeFuncs := make(map[uint64]func(sg *subgroup.SubGroup) error)

	wrap := func(wrapped func(payload []byte, pts int64) error) func(sg *subgroup.SubGroup) error {
		return func(sg *subgroup.SubGroup) error {
			ts, ok := findTimestamp(sg.Objects[0].Properties)
			if !ok {
				return fmt.Errorf("timestamp property is required")
			}

			return wrapped(sg.Objects[0].Payload, ts)
		}
	}

	for i, track := range cat.Tracks {
		trackAlias := uint64(i + 1)

		switch {
		case strings.HasPrefix(track.Codec, "av01"):
			forma := &format.AV1{
				PayloadTyp: 96,
			}
			media := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, media)

			writeFuncs[trackAlias] = wrap(func(payload []byte, pts int64) error {
				var bs av1.Bitstream
				err := bs.Unmarshal(payload)
				if err != nil {
					return err
				}

				(*subStream).WriteUnit(media, forma, &unit.Unit{PTS: pts, Payload: unit.PayloadAV1(bs)})
				return nil
			})

		case strings.HasPrefix(track.Codec, "vp09"):
			forma := &format.VP9{
				PayloadTyp: 96,
			}
			media := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, media)

			writeFuncs[trackAlias] = wrap(func(payload []byte, pts int64) error {
				(*subStream).WriteUnit(media, forma, &unit.Unit{PTS: pts, Payload: unit.PayloadVP9(payload)})
				return nil
			})

		case track.Codec == "vp8":
			forma := &format.VP8{
				PayloadTyp: 96,
			}
			media := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, media)

			writeFuncs[trackAlias] = wrap(func(payload []byte, pts int64) error {
				(*subStream).WriteUnit(media, forma, &unit.Unit{PTS: pts, Payload: unit.PayloadVP8(payload)})
				return nil
			})
		case strings.HasPrefix(track.Codec, "avc3"):
			forma := &format.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
			}
			media := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, media)

			writeFuncs[trackAlias] = wrap(func(payload []byte, pts int64) error {
				var nalus h264.AVCC
				err := nalus.Unmarshal(payload)
				if err != nil {
					return err
				}

				(*subStream).WriteUnit(media, forma, &unit.Unit{PTS: pts, Payload: unit.PayloadH264(nalus)})
				return nil
			})

		case strings.HasPrefix(track.Codec, "hev1"):
			forma := &format.H265{
				PayloadTyp: 96,
			}
			media := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{forma},
			}
			medias = append(medias, media)

			writeFuncs[trackAlias] = wrap(func(payload []byte, pts int64) error {
				var nalus h264.AVCC
				err := nalus.Unmarshal(payload)
				if err != nil {
					return err
				}

				(*subStream).WriteUnit(media, forma, &unit.Unit{PTS: pts, Payload: unit.PayloadH265(nalus)})
				return nil
			})

		case track.Codec == "opus":
			channels := track.Channels
			if channels == 0 {
				channels = 2
			}
			forma := &format.Opus{
				PayloadTyp:   96,
				ChannelCount: channels,
			}
			media := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, media)

			writeFuncs[trackAlias] = wrap(func(payload []byte, pts int64) error {
				(*subStream).WriteUnit(media, forma, &unit.Unit{PTS: pts, Payload: unit.PayloadOpus{payload}})
				return nil
			})

		case strings.HasPrefix(track.Codec, "mp4a"):
			config := &mpeg4audio.AudioSpecificConfig{
				Type:          mpeg4audio.ObjectTypeAACLC,
				SampleRate:    track.Samplerate,
				ChannelConfig: uint8(track.Channels),
			}

			forma := &format.MPEG4Audio{
				PayloadTyp:       96,
				Config:           config,
				SizeLength:       13,
				IndexLength:      3,
				IndexDeltaLength: 3,
			}
			media := &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{forma},
			}
			medias = append(medias, media)

			writeFuncs[trackAlias] = wrap(func(payload []byte, pts int64) error {
				(*subStream).WriteUnit(media, forma, &unit.Unit{PTS: pts, Payload: unit.PayloadMPEG4Audio{payload}})
				return nil
			})

		default:
			return nil, nil, fmt.Errorf("unsupported codec: %s", track.Codec)
		}
	}

	return medias, writeFuncs, nil
}
