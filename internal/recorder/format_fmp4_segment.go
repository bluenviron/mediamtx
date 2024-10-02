package recorder

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/pkg/formats/fmp4/seekablebuffer"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

func writeInit(f io.Writer, tracks []*formatFMP4Track) error {
	fmp4Tracks := make([]*fmp4.InitTrack, len(tracks))
	for i, track := range tracks {
		fmp4Tracks[i] = track.initTrack
	}

	init := fmp4.Init{
		Tracks: fmp4Tracks,
	}

	var buf seekablebuffer.Buffer
	err := init.Marshal(&buf)
	if err != nil {
		return err
	}

	_, err = f.Write(buf.Bytes())
	return err
}

type formatFMP4Segment struct {
	f        *formatFMP4
	startDTS time.Duration
	startNTP time.Time

	path    string
	fi      *os.File
	curPart *formatFMP4Part
	lastDTS time.Duration

	tsFi *os.File
}

func (s *formatFMP4Segment) initialize() {
	s.lastDTS = s.startDTS
}

func (s *formatFMP4Segment) close() error {
	var err error

	if s.curPart != nil {
		err = s.curPart.close()
	}

	if s.fi != nil {
		s.f.ai.Log(logger.Debug, "closing segment %s", s.path)
		err2 := s.fi.Close()
		if err == nil {
			err = err2
		}

		if err2 == nil {
			duration := s.lastDTS - s.startDTS
			s.f.ai.agent.OnSegmentComplete(s.path, duration)
		}
	}

	if s.tsFi != nil {
		s.tsFi.Close()
	}

	return err
}

func (s *formatFMP4Segment) write(track *formatFMP4Track, sample *sample) error {
	s.lastDTS = sample.dts

	if s.curPart == nil {
		s.curPart = &formatFMP4Part{
			s:              s,
			sequenceNumber: s.f.nextSequenceNumber,
			startDTS:       sample.dts,
		}
		s.curPart.initialize()
		s.f.nextSequenceNumber++
	} else if s.curPart.duration() >= s.f.ai.agent.PartDuration {
		err := s.curPart.close()
		s.curPart = nil

		if err != nil {
			return err
		}

		s.curPart = &formatFMP4Part{
			s:              s,
			sequenceNumber: s.f.nextSequenceNumber,
			startDTS:       sample.dts,
		}
		s.curPart.initialize()
		s.f.nextSequenceNumber++
	}

	if s.tsFi == nil {
		path := recordstore.Path{Start: s.startNTP}.Encode(s.f.ai.pathFormat)
		path = strings.Replace(path, ".mp4", ".csv", 1)
		fi, err := os.Create(path)
		if err != nil {
			s.tsFi = fi
		}
	} else {
		s.tsFi.WriteString(sample.ntp.Format("2006-01-02T15:04:05.000000Z,\n"))
	}

	return s.curPart.write(track, sample)
}
