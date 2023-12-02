package record

import (
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type formatMPEGTSSegment struct {
	f         *formatMPEGTS
	startDTS  time.Duration
	lastFlush time.Duration

	created time.Time
	fpath   string
	fi      *os.File
}

func newFormatMPEGTSSegment(f *formatMPEGTS, startDTS time.Duration) *formatMPEGTSSegment {
	s := &formatMPEGTSSegment{
		f:         f,
		startDTS:  startDTS,
		lastFlush: startDTS,
		created:   timeNow(),
	}

	f.dw.setTarget(s)

	return s
}

func (s *formatMPEGTSSegment) close() error {
	err := s.f.bw.Flush()

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

func (s *formatMPEGTSSegment) Write(p []byte) (int, error) {
	if s.fi == nil {
		s.fpath = encodeRecordPath(&recordPathParams{time: s.created}, s.f.a.resolvedPath)
		s.f.a.wrapper.Log(logger.Debug, "creating segment %s", s.fpath)

		err := os.MkdirAll(filepath.Dir(s.fpath), 0o755)
		if err != nil {
			return 0, err
		}

		fi, err := os.Create(s.fpath)
		if err != nil {
			return 0, err
		}

		s.f.a.wrapper.OnSegmentCreate(s.fpath)

		s.fi = fi
	}

	return s.fi.Write(p)
}
