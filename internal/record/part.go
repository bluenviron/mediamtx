package record

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aler9/writerseeker"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/logger"
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
	if p.s.f == nil {
		p.s.fpath = encodeRecordPath(&recordPathParams{time: timeNow()}, p.s.r.path)
		p.s.r.Log(logger.Debug, "opening segment %s", p.s.fpath)

		err := os.MkdirAll(filepath.Dir(p.s.fpath), 0o755)
		if err != nil {
			return err
		}

		f, err := os.Create(p.s.fpath)
		if err != nil {
			return err
		}

		err = writeInit(f, p.s.r.tracks)
		if err != nil {
			f.Close()
			return err
		}

		p.s.f = f
	}

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
