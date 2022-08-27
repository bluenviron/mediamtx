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

	isIndependent    bool
	videoSamples     []*fmp4.VideoSample
	audioSamples     []*fmp4.AudioSample
	content          []byte
	renderedDuration time.Duration
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
		ret := time.Duration(0)
		for _, e := range p.videoSamples {
			ret += e.Duration()
		}
		return ret
	}

	// use the sum of the default duration of all samples,
	// not the real duration,
	// otherwise on iPhone iOS the stream freezes.
	return time.Duration(len(p.audioSamples)) * time.Second *
		time.Duration(mpeg4audio.SamplesPerAccessUnit) / time.Duration(p.audioTrack.ClockRate())
}

func (p *muxerVariantFMP4Part) finalize() error {
	if len(p.videoSamples) > 0 || len(p.audioSamples) > 0 {
		var err error
		p.content, err = fmp4.GeneratePart(
			p.videoTrack,
			p.audioTrack,
			p.videoSamples,
			p.audioSamples)
		if err != nil {
			return err
		}

		p.renderedDuration = p.duration()
	}

	p.videoSamples = nil
	p.audioSamples = nil

	return nil
}

func (p *muxerVariantFMP4Part) writeH264(sample *fmp4.VideoSample) {
	if sample.IDRPresent {
		p.isIndependent = true
	}
	p.videoSamples = append(p.videoSamples, sample)
}

func (p *muxerVariantFMP4Part) writeAAC(sample *fmp4.AudioSample) {
	p.audioSamples = append(p.audioSamples, sample)
}
