package core

import (
	"context"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type srtSourceParent interface {
	logger.Writer
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
}

type srtSource struct {
	readTimeout conf.StringDuration
	parent      srtSourceParent
}

func newSRTSource(
	readTimeout conf.StringDuration,
	parent srtSourceParent,
) *srtSource {
	s := &srtSource{
		readTimeout: readTimeout,
		parent:      parent,
	}

	return s
}

func (s *srtSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[SRT source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *srtSource) run(ctx context.Context, cnf *conf.Path, reloadConf chan *conf.Path) error {
	s.Log(logger.Debug, "connecting")

	conf := srt.DefaultConfig()
	address, err := conf.UnmarshalURL(cnf.Source)
	if err != nil {
		return err
	}

	err = conf.Validate()
	if err != nil {
		return err
	}

	sconn, err := srt.Dial("srt", address, conf)
	if err != nil {
		return err
	}

	readDone := make(chan error)
	go func() {
		readDone <- s.runReader(sconn)
	}()

	for {
		select {
		case err := <-readDone:
			sconn.Close()
			return err

		case <-reloadConf:

		case <-ctx.Done():
			sconn.Close()
			<-readDone
			return nil
		}
	}
}

func (s *srtSource) runReader(sconn srt.Conn) error {
	sconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
	r, err := mpegts.NewReader(mpegts.NewBufferedReader(sconn))
	if err != nil {
		return err
	}

	decodeErrLogger := logger.NewLimitedLogger(s)

	r.OnDecodeError(func(err error) {
		decodeErrLogger.Log(logger.Warn, err.Error())
	})

	var stream *stream.Stream

	medias, err := mpegtsSetupTracks(r, &stream)
	if err != nil {
		return err
	}

	res := s.parent.setReady(pathSourceStaticSetReadyReq{
		desc:               &description.Session{Medias: medias},
		generateRTPPackets: true,
	})
	if res.err != nil {
		return res.err
	}

	stream = res.stream

	for {
		sconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*srtSource) apiSourceDescribe() apiPathSourceOrReader {
	return apiPathSourceOrReader{
		Type: "srtSource",
		ID:   "",
	}
}
