package rtmp

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// ToStream maps a RTMP stream to a MediaMTX stream.
func ToStream(r *Reader, stream **stream.Stream) ([]*description.Media, error) {
	videoFormat, audioFormat := r.Tracks()

	var medias []*description.Media

	if videoFormat != nil {
		medi := &description.Media{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{videoFormat},
		}
		medias = append(medias, medi)

		switch videoFormat.(type) {
		case *format.AV1:
			r.OnDataAV1(func(pts time.Duration, tu [][]byte) {
				(*stream).WriteUnit(medi, videoFormat, &unit.AV1{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					TU: tu,
				})
			})

		case *format.VP9:
			r.OnDataVP9(func(pts time.Duration, frame []byte) {
				(*stream).WriteUnit(medi, videoFormat, &unit.VP9{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Frame: frame,
				})
			})

		case *format.H265:
			r.OnDataH265(func(pts time.Duration, au [][]byte) {
				(*stream).WriteUnit(medi, videoFormat, &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AU: au,
				})
			})

		case *format.H264:
			r.OnDataH264(func(pts time.Duration, au [][]byte) {
				(*stream).WriteUnit(medi, videoFormat, &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AU: au,
				})
			})

		default:
			return nil, fmt.Errorf("unsupported video codec: %T", videoFormat)
		}
	}

	if audioFormat != nil {
		medi := &description.Media{
			Type:    description.MediaTypeAudio,
			Formats: []format.Format{audioFormat},
		}
		medias = append(medias, medi)

		switch audioFormat.(type) {
		case *format.MPEG4Audio:
			r.OnDataMPEG4Audio(func(pts time.Duration, au []byte) {
				(*stream).WriteUnit(medi, audioFormat, &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AUs: [][]byte{au},
				})
			})

		case *format.MPEG1Audio:
			r.OnDataMPEG1Audio(func(pts time.Duration, frame []byte) {
				(*stream).WriteUnit(medi, audioFormat, &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Frames: [][]byte{frame},
				})
			})

		case *format.G711:
			r.OnDataG711(func(pts time.Duration, samples []byte) {
				(*stream).WriteUnit(medi, audioFormat, &unit.G711{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Samples: samples,
				})
			})

		case *format.LPCM:
			r.OnDataLPCM(func(pts time.Duration, samples []byte) {
				(*stream).WriteUnit(medi, audioFormat, &unit.LPCM{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Samples: samples,
				})
			})

		default:
			return nil, fmt.Errorf("unsupported audio codec: %T", audioFormat)
		}
	}

	return medias, nil
}
