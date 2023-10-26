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

func writePart(
	f io.Writer,
	sequenceNumber uint32,
	partTracks map[*recFormatFMP4Track]*fmp4.PartTrack,
) error {
	fmp4PartTracks := make([]*fmp4.PartTrack, len(partTracks))
	i := 0
	for _, partTrack := range partTracks {
		fmp4PartTracks[i] = partTrack
		i++
	}

	part := &fmp4.Part{
		SequenceNumber: sequenceNumber,
		Tracks:         fmp4PartTracks,
	}

	var ws writerseeker.WriterSeeker
	err := part.Marshal(&ws)
	if err != nil {
		return err
	}

	_, err = f.Write(ws.Bytes())
	return err
}

type recFormatFMP4Part struct {
	s              *recFormatFMP4Segment
	sequenceNumber uint32
	startDTS       time.Duration

	created    time.Time
	partTracks map[*recFormatFMP4Track]*fmp4.PartTrack
	endDTS     time.Duration
}

func newRecFormatFMP4Part(
	s *recFormatFMP4Segment,
	sequenceNumber uint32,
	startDTS time.Duration,
) *recFormatFMP4Part {
	return &recFormatFMP4Part{
		s:              s,
		startDTS:       startDTS,
		sequenceNumber: sequenceNumber,
		created:        timeNow(),
		partTracks:     make(map[*recFormatFMP4Track]*fmp4.PartTrack),
	}
}

func (p *recFormatFMP4Part) close() error {
	if p.s.fi == nil {
		p.s.fpath = encodeRecordPath(&recordPathParams{time: p.created}, p.s.f.a.resolvedPath)
		p.s.f.a.wrapper.Log(logger.Debug, "creating segment %s", p.s.fpath)

		err := os.MkdirAll(filepath.Dir(p.s.fpath), 0o755)
		if err != nil {
			return err
		}

		fi, err := os.Create(p.s.fpath)
		if err != nil {
			return err
		}

		p.s.f.a.wrapper.OnSegmentCreate(p.s.fpath)

		err = writeInit(fi, p.s.f.tracks)
		if err != nil {
			fi.Close()
			return err
		}

		p.s.fi = fi
	}

	return writePart(p.s.fi, p.sequenceNumber, p.partTracks)
}

func (p *recFormatFMP4Part) record(track *recFormatFMP4Track, sample *sample) error {
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

func (p *recFormatFMP4Part) duration() time.Duration {
	return p.endDTS - p.startDTS
}
