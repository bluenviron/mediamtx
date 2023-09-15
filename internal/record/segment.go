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

	fpath       string
	f           *os.File
	initWritten bool
	curPart     *part
}

func newSegment(
	r *Agent,
	startDTS time.Duration,
) (*segment, error) {
	s := &segment{
		r:        r,
		startDTS: startDTS,
	}

	s.fpath = encodeRecordPath(&recordPathParams{time: time.Now()}, r.path)

	r.Log(logger.Debug, "opening segment %s", s.fpath)

	err := os.MkdirAll(filepath.Dir(s.fpath), 0o755)
	if err != nil {
		return nil, err
	}

	s.f, err = os.Create(s.fpath)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *segment) close() error {
	s.r.Log(logger.Debug, "closing segment %s", s.fpath)

	// write init file at the last moment
	// in order to allow codec parameters to be overridden
	if !s.initWritten {
		s.initWritten = true
		err := writeInit(s.f, s.r.tracks)
		if err != nil {
			s.f.Close()
			return err
		}
	}

	err := s.curPart.close()
	if err != nil {
		s.f.Close()
		return err
	}

	return s.f.Close()
}

func (s *segment) record(track *track, sample *sample) error {
	if s.curPart == nil {
		s.curPart = newPart(s, sample.dts)
	} else if s.curPart.duration() >= s.r.partDuration {
		// write init file at the last moment
		// in order to allow codec parameters to be overridden
		if !s.initWritten {
			s.initWritten = true
			err := writeInit(s.f, s.r.tracks)
			if err != nil {
				return err
			}
		}

		err := s.curPart.close()
		if err != nil {
			return err
		}

		s.curPart = newPart(s, sample.dts)
	}

	return s.curPart.record(track, sample)
}
