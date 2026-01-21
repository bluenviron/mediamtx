// Package mpegts contains MPEG-ts utilities.
package mpegts

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"H265, H264, MPEG-4 Video, MPEG-1/2 Video, Opus, MPEG-4 Audio, MPEG-1 Audio, AC-3")

// ToStream maps a MPEG-TS stream to a MediaMTX stream.
func ToStream(
	r *EnhancedReader,
	strm **stream.Stream,
	l logger.Writer,
) ([]*description.Media, error) {
	var medias []*description.Media //nolint:prealloc
	var unsupportedTracks []int

	td := &mpegts.TimeDecoder{}
	td.Initialize()

	for i, track := range r.Tracks() { //nolint:dupl
		var medi *description.Media

		switch codec := track.Codec.(type) {
		case *tscodecs.H265:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
				}},
			}

			r.OnDataH265(track, func(pts int64, _ int64, au [][]byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					Payload: unit.PayloadH265(au),
				})
				return nil
			})

		case *tscodecs.H264:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			r.OnDataH264(track, func(pts int64, _ int64, au [][]byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					Payload: unit.PayloadH264(au),
				})
				return nil
			})

		case *tscodecs.MPEG4Video:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG4Video{
					PayloadTyp: 96,
				}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					Payload: unit.PayloadMPEG4Video(frame),
				})
				return nil
			})

		case *tscodecs.MPEG1Video:
			medi = &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG1Video{}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					Payload: unit.PayloadMPEG1Video(frame),
				})
				return nil
			})

		case *tscodecs.Opus:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: codec.ChannelCount,
				}},
			}

			r.OnDataOpus(track, func(pts int64, packets [][]byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					Payload: unit.PayloadOpus(packets),
				})
				return nil
			})

		case *tscodecs.KLV:
			medi = &description.Media{
				Type: description.MediaTypeApplication,
				Formats: []format.Format{&format.KLV{
					PayloadTyp: 96,
				}},
			}
			r.OnDataKLV(track, func(pts int64, uni []byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     pts,
					Payload: unit.PayloadKLV(uni),
				})
				return nil
			})

		case *tscodecs.MPEG4Audio:
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
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					Payload: unit.PayloadMPEG4Audio(aus),
				})
				return nil
			})

		case *tscodecs.MPEG4AudioLATM:
			// We are dealing with a LATM stream with in-band configuration.
			// Although in theory this can be streamed with RTSP (RFC6416 with cpresent=1),
			// in practice there is no player that supports it.
			// Therefore, convert the stream to a LATM stream with out-of-band configuration.
			streamMuxConfig := r.latmConfigs[track.PID]
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4AudioLATM{
					PayloadTyp:      96,
					CPresent:        false,
					ProfileLevelID:  30,
					StreamMuxConfig: streamMuxConfig,
				}},
			}
			clockRate := medi.Formats[0].ClockRate()

			r.OnDataMPEG4AudioLATM(track, func(pts int64, els [][]byte) error {
				pts = td.Decode(pts)

				pts = multiplyAndDivide(pts, int64(clockRate), 90000)

				for _, el := range els {
					var elIn mpeg4audio.AudioMuxElement
					elIn.MuxConfigPresent = true
					elIn.StreamMuxConfig = streamMuxConfig
					err := elIn.Unmarshal(el)
					if err != nil {
						return err
					}

					if !reflect.DeepEqual(elIn.StreamMuxConfig, streamMuxConfig) {
						return fmt.Errorf("dynamic stream mux config is not supported")
					}

					var elOut mpeg4audio.AudioMuxElement
					elOut.MuxConfigPresent = false
					elOut.StreamMuxConfig = streamMuxConfig
					elOut.Payloads = elIn.Payloads
					buf, err := elOut.Marshal()
					if err != nil {
						return err
					}

					(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
						PTS:     pts,
						Payload: unit.PayloadMPEG4AudioLATM(buf),
					})

					pts += mpeg4audio.SamplesPerAccessUnit
				}

				return nil
			})

		case *tscodecs.MPEG1Audio:
			medi = &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG1Audio{}},
			}

			r.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					Payload: unit.PayloadMPEG1Audio(frames),
				})
				return nil
			})

		case *tscodecs.AC3:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.AC3{
					PayloadTyp:   96,
					SampleRate:   codec.SampleRate,
					ChannelCount: codec.ChannelCount,
				}},
			}

			r.OnDataAC3(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*strm).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					PTS:     multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					Payload: unit.PayloadAC3{frame},
				})
				return nil
			})

		default:
			unsupportedTracks = append(unsupportedTracks, i+1)
			continue
		}

		medias = append(medias, medi)
	}

	if len(medias) == 0 {
		return nil, errNoSupportedCodecs
	}

	for _, id := range unsupportedTracks {
		l.Log(logger.Warn, "skipping track %d (unsupported codec)", id)
	}

	return medias, nil
}
