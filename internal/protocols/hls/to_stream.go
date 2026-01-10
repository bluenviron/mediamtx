package hls

import (
	"sync"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/ntpestimator"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type ntpState int

const (
	ntpStateInitial ntpState = iota
	ntpStateUnavailable
	ntpStateAvailable
	ntpStateReplace
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
	pathConf *conf.Path,
	subStream **stream.SubStream,
) ([]*description.Media, error) {
	var ntpStat ntpState
	var ntpStatMutex sync.Mutex

	if !pathConf.UseAbsoluteTimestamp {
		ntpStat = ntpStateReplace
	}

	var medias []*description.Media //nolint:prealloc

	for _, track := range tracks {
		ctrack := track
		ntpEstimator := &ntpestimator.Estimator{ClockRate: track.ClockRate}

		handleNTP := func(pts int64) time.Time {
			ntpStatMutex.Lock()
			defer ntpStatMutex.Unlock()

			switch ntpStat {
			case ntpStateInitial:
				ntp, avail := c.AbsoluteTime(ctrack)
				if !avail {
					ntpStat = ntpStateUnavailable
					return ntpEstimator.Estimate(pts)
				}
				ntpStat = ntpStateAvailable
				return ntp

			case ntpStateAvailable:
				ntp, avail := c.AbsoluteTime(ctrack)
				if !avail {
					panic("should not happen")
				}
				return ntp

			case ntpStateUnavailable:
				_, avail := c.AbsoluteTime(ctrack)
				if avail {
					// absolute timestamp appeared after stream started, we are not using it
					ntpStat = ntpStateReplace
				}
				return ntpEstimator.Estimate(pts)

			default: // ntpStateReplace
				return ntpEstimator.Estimate(pts)
			}
		}

		var medi *description.Media

		switch tcodec := ctrack.Codec.(type) {
		case *codecs.AV1:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.AV1{
					PayloadTyp: 96,
				}},
			}
			newClockRate := medi.Formats[0].ClockRate()

			c.OnDataAV1(ctrack, func(pts int64, tu [][]byte) {
				(*subStream).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					NTP:     handleNTP(pts),
					PTS:     multiplyAndDivide(pts, int64(newClockRate), int64(ctrack.ClockRate)),
					Payload: unit.PayloadAV1(tu),
				})
			})

		case *codecs.VP9:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.VP9{
					PayloadTyp: 96,
				}},
			}
			newClockRate := medi.Formats[0].ClockRate()

			c.OnDataVP9(ctrack, func(pts int64, frame []byte) {
				(*subStream).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					NTP:     handleNTP(pts),
					PTS:     multiplyAndDivide(pts, int64(newClockRate), int64(ctrack.ClockRate)),
					Payload: unit.PayloadVP9(frame),
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
			newClockRate := medi.Formats[0].ClockRate()

			c.OnDataH26x(ctrack, func(pts int64, _ int64, au [][]byte) {
				(*subStream).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					NTP:     handleNTP(pts),
					PTS:     multiplyAndDivide(pts, int64(newClockRate), int64(ctrack.ClockRate)),
					Payload: unit.PayloadH265(au),
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
			newClockRate := medi.Formats[0].ClockRate()

			c.OnDataH26x(ctrack, func(pts int64, _ int64, au [][]byte) {
				(*subStream).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					NTP:     handleNTP(pts),
					PTS:     multiplyAndDivide(pts, int64(newClockRate), int64(ctrack.ClockRate)),
					Payload: unit.PayloadH264(au),
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
			newClockRate := medi.Formats[0].ClockRate()

			c.OnDataOpus(ctrack, func(pts int64, packets [][]byte) {
				(*subStream).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					NTP:     handleNTP(pts),
					PTS:     multiplyAndDivide(pts, int64(newClockRate), int64(ctrack.ClockRate)),
					Payload: unit.PayloadOpus(packets),
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
			newClockRate := medi.Formats[0].ClockRate()

			c.OnDataMPEG4Audio(ctrack, func(pts int64, aus [][]byte) {
				(*subStream).WriteUnit(medi, medi.Formats[0], &unit.Unit{
					NTP:     handleNTP(pts),
					PTS:     multiplyAndDivide(pts, int64(newClockRate), int64(ctrack.ClockRate)),
					Payload: unit.PayloadMPEG4Audio(aus),
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
