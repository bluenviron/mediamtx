package record

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aler9/writerseeker"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
)

func writeInit(f io.Writer, tracks []*track) error {
	fmp4Tracks := make([]*fmp4.InitTrack, len(tracks))
	for i, track := range tracks {
		fmp4Tracks[i] = track.initTrack
	}

	init := fmp4.Init{
		Tracks: fmp4Tracks,
	}

	var ws writerseeker.WriterSeeker
	err := init.Marshal(&ws)
	if err != nil {
		return err
	}

	_, err = f.Write(ws.Bytes())
	return err
}

type segment struct {
	r        *Recorder
	startDTS time.Duration

	f       *os.File
	curPart *part
}

func newSegment(
	r *Recorder,
	startDTS time.Duration,
) (*segment, error) {
	s := &segment{
		r:        r,
		startDTS: startDTS,
	}

	fpath := encodeRecordPath(&recordPathParams{time: time.Now()}, r.path)

	err := os.MkdirAll(filepath.Dir(fpath), 0o755)
	if err != nil {
		return nil, err
	}

	s.f, err = os.Create(fpath)
	if err != nil {
		return nil, err
	}

	err = writeInit(s.f, r.tracks)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *segment) record(track *track, sample *sample) error {
	if s.curPart == nil {
		s.curPart = newPart(s, sample.dts)
	} else if s.curPart.duration() >= s.r.partDuration {
		err := s.curPart.close()
		if err != nil {
			return err
		}
		s.curPart = newPart(s, sample.dts)
	}

	return s.curPart.record(track, sample)
}

func (s *segment) close() error {
	err := s.curPart.close()
	if err != nil {
		return err
	}

	return s.f.Close()
}
