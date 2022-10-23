package hls

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/mpeg4audio"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
)

func fmp4PartName(id uint64) string {
	return "part" + strconv.FormatUint(id, 10)
}

type muxerVariantFMP4Part struct {
	videoTrack *gortsplib.TrackH264
	audioTrack *gortsplib.TrackMPEG4Audio
	id         uint64

	isIndependent       bool
	videoSamples        []*fmp4.PartSample
	audioSamples        []*fmp4.PartSample
	content             []byte
	renderedDuration    time.Duration
	videoStartDTSFilled bool
	videoStartDTS       time.Duration
	audioStartDTSFilled bool
	audioStartDTS       time.Duration
}

func newMuxerVariantFMP4Part(
	videoTrack *gortsplib.TrackH264,
	audioTrack *gortsplib.TrackMPEG4Audio,
	id uint64,
) *muxerVariantFMP4Part {
	p := &muxerVariantFMP4Part{
		videoTrack: videoTrack,
		audioTrack: audioTrack,
		id:         id,
	}

	if videoTrack == nil {
		p.isIndependent = true
	}

	return p
}

func (p *muxerVariantFMP4Part) name() string {
	return fmp4PartName(p.id)
}

func (p *muxerVariantFMP4Part) reader() io.Reader {
	return bytes.NewReader(p.content)
}

func (p *muxerVariantFMP4Part) duration() time.Duration {
	if p.videoTrack != nil {
		ret := uint64(0)
		for _, e := range p.videoSamples {
			ret += uint64(e.Duration)
		}
		return durationMp4ToGo(ret, 90000)
	}

	// use the sum of the default duration of all samples,
	// not the real duration,
	// otherwise on iPhone iOS the stream freezes.
	return time.Duration(len(p.audioSamples)) * time.Second *
		time.Duration(mpeg4audio.SamplesPerAccessUnit) / time.Duration(p.audioTrack.ClockRate())
}

func (p *muxerVariantFMP4Part) finalize() error {
	if p.videoSamples != nil || p.audioSamples != nil {
		part := fmp4.Part{}

		if p.videoSamples != nil {
			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       1,
				BaseTime: durationGoToMp4(p.videoStartDTS, 90000),
				Samples:  p.videoSamples,
				IsVideo:  true,
			})
		}

		if p.audioSamples != nil {
			var id int
			if p.videoTrack != nil {
				id = 2
			} else {
				id = 1
			}

			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       id,
				BaseTime: durationGoToMp4(p.audioStartDTS, uint32(p.audioTrack.ClockRate())),
				Samples:  p.audioSamples,
			})
		}

		var err error
		p.content, err = part.Marshal()
		if err != nil {
			return err
		}

		p.renderedDuration = p.duration()
	}

	p.videoSamples = nil
	p.audioSamples = nil

	return nil
}

func (p *muxerVariantFMP4Part) writeH264(sample *augmentedVideoSample) {
	if !p.videoStartDTSFilled {
		p.videoStartDTSFilled = true
		p.videoStartDTS = sample.dts
	}

	if (sample.Flags & (1 << 16)) == 0 {
		p.isIndependent = true
	}

	p.videoSamples = append(p.videoSamples, &sample.PartSample)
}

func (p *muxerVariantFMP4Part) writeAAC(sample *augmentedAudioSample) {
	if !p.audioStartDTSFilled {
		p.audioStartDTSFilled = true
		p.audioStartDTS = sample.dts
	}

	p.audioSamples = append(p.audioSamples, &sample.PartSample)
}
