package record

import (
	"io"
	"os"
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
	var err error

	if s.curPart != nil {
		err = s.curPart.close()
	}

	if s.f != nil {
		s.r.Log(logger.Debug, "closing segment %s", s.fpath)
		err2 := s.f.Close()
		if err == nil {
			err = err2
		}

		if err2 == nil {
			s.r.onSegmentComplete(s.fpath)
		}
	}

	return err
}

func (s *segment) record(track *track, sample *sample) error {
	if s.curPart == nil {
		s.curPart = newPart(s, sample.dts)
	} else if s.curPart.duration() >= s.r.partDuration {
		err := s.curPart.close()
		s.curPart = nil

		if err != nil {
			return err
		}

		s.curPart = newPart(s, sample.dts)
	}

	return s.curPart.record(track, sample)
}
