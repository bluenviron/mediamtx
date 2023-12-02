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

func writeInit(f io.Writer, tracks []*formatFMP4Track) error {
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

type formatFMP4Segment struct {
	f        *formatFMP4
	startDTS time.Duration

	fpath   string
	fi      *os.File
	curPart *formatFMP4Part
}

func newFormatFMP4Segment(
	f *formatFMP4,
	startDTS time.Duration,
) *formatFMP4Segment {
	return &formatFMP4Segment{
		f:        f,
		startDTS: startDTS,
	}
}

func (s *formatFMP4Segment) close() error {
	var err error

	if s.curPart != nil {
		err = s.curPart.close()
	}

	if s.fi != nil {
		s.f.a.wrapper.Log(logger.Debug, "closing segment %s", s.fpath)
		err2 := s.fi.Close()
		if err == nil {
			err = err2
		}

		if err2 == nil {
			s.f.a.wrapper.OnSegmentComplete(s.fpath)
		}
	}

	return err
}

func (s *formatFMP4Segment) record(track *formatFMP4Track, sample *sample) error {
	if s.curPart == nil {
		s.curPart = newFormatFMP4Part(s, s.f.nextSequenceNumber, sample.dts)
		s.f.nextSequenceNumber++
	} else if s.curPart.duration() >= s.f.a.wrapper.PartDuration {
		err := s.curPart.close()
		s.curPart = nil

		if err != nil {
			return err
		}

		s.curPart = newFormatFMP4Part(s, s.f.nextSequenceNumber, sample.dts)
		s.f.nextSequenceNumber++
	}

	return s.curPart.record(track, sample)
}
