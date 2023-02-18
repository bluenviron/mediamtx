package core

import (
	"context"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/bluenviron/gohlslib"
)

type hlsSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type hlsSource struct {
	parent hlsSourceParent
}

func newHLSSource(
	parent hlsSourceParent,
) *hlsSource {
	return &hlsSource{
		parent: parent,
	}
}

func (s *hlsSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[hls source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *hlsSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	var stream *stream

	defer func() {
		if stream != nil {
			s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	c := &gohlslib.Client{
		URI:         cnf.Source,
		Fingerprint: cnf.SourceFingerprint,
		Log: func(level gohlslib.LogLevel, format string, args ...interface{}) {
			s.Log(logger.Level(level), format, args...)
		},
	}

	c.OnTracks(func(tracks []format.Format) error {
		var medias media.Medias

		for _, track := range tracks {
			medi := &media.Media{
				Formats: []format.Format{track},
			}
			medias = append(medias, medi)
			cformat := track

			switch track.(type) {
			case *format.H264:
				medi.Type = media.TypeVideo

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, cformat, &formatprocessor.UnitH264{
						PTS: pts,
						AU:  unit.([][]byte),
						NTP: time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})

			case *format.H265:
				medi.Type = media.TypeVideo

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, cformat, &formatprocessor.UnitH265{
						PTS: pts,
						AU:  unit.([][]byte),
						NTP: time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})

			case *format.MPEG4Audio:
				medi.Type = media.TypeAudio

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, cformat, &formatprocessor.UnitMPEG4Audio{
						PTS: pts,
						AUs: [][]byte{unit.([]byte)},
						NTP: time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})

			case *format.Opus:
				medi.Type = media.TypeAudio

				c.OnData(track, func(pts time.Duration, unit interface{}) {
					err := stream.writeData(medi, cformat, &formatprocessor.UnitOpus{
						PTS:   pts,
						Frame: unit.([]byte),
						NTP:   time.Now(),
					})
					if err != nil {
						s.Log(logger.Warn, "%v", err)
					}
				})
			}
		}

		res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
			medias:             medias,
			generateRTPPackets: true,
		})
		if res.err != nil {
			return res.err
		}

		s.Log(logger.Info, "ready: %s", sourceMediaInfo(medias))
		stream = res.stream

		return nil
	})

	err := c.Start()
	if err != nil {
		return err
	}

	for {
		select {
		case err := <-c.Wait():
			c.Close()
			return err

		case <-reloadConf:

		case <-ctx.Done():
			c.Close()
			<-c.Wait()
			return nil
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*hlsSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsSource"}
}
