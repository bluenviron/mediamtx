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

var timeNow = time.Now

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
	r        *Agent
	startDTS time.Duration

	fpath   string
	f       *os.File
	curPart *part
}

func newSegment(
	r *Agent,
	startDTS time.Duration,
) *segment {
	return &segment{
		r:        r,
		startDTS: startDTS,
	}
}

func (s *segment) close() error {
	if s.curPart != nil {
		err := s.flush()

		if s.f != nil {
			s.r.Log(logger.Debug, "closing segment %s", s.fpath)

			err2 := s.f.Close()
			if err == nil {
				err = err2
			}
		}

		return err
	}

	return nil
}

func (s *segment) record(track *track, sample *sample) error {
	if s.curPart == nil {
		s.curPart = newPart(s, sample.dts)
	} else if s.curPart.duration() >= s.r.partDuration {
		err := s.flush()
		if err != nil {
			s.curPart = nil
			return err
		}

		s.curPart = newPart(s, sample.dts)
	}

	return s.curPart.record(track, sample)
}

func (s *segment) flush() error {
	if s.f == nil {
		s.fpath = encodeRecordPath(&recordPathParams{time: timeNow()}, s.r.path)
		s.r.Log(logger.Debug, "opening segment %s", s.fpath)

		err := os.MkdirAll(filepath.Dir(s.fpath), 0o755)
		if err != nil {
			return err
		}

		f, err := os.Create(s.fpath)
		if err != nil {
			return err
		}

		err = writeInit(f, s.r.tracks)
		if err != nil {
			f.Close()
			return err
		}

		s.f = f
	}

	return s.curPart.close()
}
