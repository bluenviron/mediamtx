package record

import (
	"io"
	"time"

	"github.com/aler9/writerseeker"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

func writePart(f io.Writer, partTracks map[*track]*fmp4.PartTrack) error {
	fmp4PartTracks := make([]*fmp4.PartTrack, len(partTracks))
	i := 0
	for _, partTrack := range partTracks {
		fmp4PartTracks[i] = partTrack
		i++
	}

	part := &fmp4.Part{
		Tracks: fmp4PartTracks,
	}

	var ws writerseeker.WriterSeeker
	err := part.Marshal(&ws)
	if err != nil {
		return err
	}

	_, err = f.Write(ws.Bytes())
	return err
}

type part struct {
	s        *segment
	startDTS time.Duration

	partTracks map[*track]*fmp4.PartTrack
	endDTS     time.Duration
}

func newPart(
	s *segment,
	startDTS time.Duration,
) *part {
	return &part{
		s:          s,
		startDTS:   startDTS,
		partTracks: make(map[*track]*fmp4.PartTrack),
	}
}

func (p *part) close() error {
	return writePart(p.s.f, p.partTracks)
}

func (p *part) record(track *track, sample *sample) error {
	partTrack, ok := p.partTracks[track]
	if !ok {
		partTrack = &fmp4.PartTrack{
			ID:       track.initTrack.ID,
			BaseTime: durationGoToMp4(sample.dts-p.s.startDTS, track.initTrack.TimeScale),
		}
		p.partTracks[track] = partTrack
	}

	partTrack.Samples = append(partTrack.Samples, sample.PartSample)
	p.endDTS = sample.dts

	return nil
}

func (p *part) duration() time.Duration {
	return p.endDTS - p.startDTS
}
