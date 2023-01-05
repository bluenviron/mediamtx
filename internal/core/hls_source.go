package core

import (
	"context"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"

	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type hlsSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type hlsSource struct {
	ur          string
	fingerprint string
	parent      hlsSourceParent
}

func newHLSSource(
	ur string,
	fingerprint string,
	parent hlsSourceParent,
) *hlsSource {
	return &hlsSource{
		ur:          ur,
		fingerprint: fingerprint,
		parent:      parent,
	}
}

func (s *hlsSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[hls source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *hlsSource) run(ctx context.Context) error {
	var stream *stream
	var videoMedia *media.Media
	var audioMedia *media.Media

	defer func() {
		if stream != nil {
			s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
		}
	}()

	onTracks := func(videoFormat *format.H264, audioFormat *format.MPEG4Audio) error {
		var medias media.Medias

		if videoFormat != nil {
			videoMedia = &media.Media{
				Type:    media.TypeVideo,
				Formats: []format.Format{videoFormat},
			}
			medias = append(medias, videoMedia)
		}

		if audioFormat != nil {
			audioMedia = &media.Media{
				Type:    media.TypeAudio,
				Formats: []format.Format{audioFormat},
			}
			medias = append(medias, audioMedia)
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
	}

	onVideoData := func(pts time.Duration, au [][]byte) {
		err := stream.writeData(videoMedia, videoMedia.Formats[0], &formatprocessor.DataH264{
			PTS: pts,
			AU:  au,
			NTP: time.Now(),
		})
		if err != nil {
			s.Log(logger.Warn, "%v", err)
		}
	}

	onAudioData := func(pts time.Duration, au []byte) {
		err := stream.writeData(audioMedia, audioMedia.Formats[0], &formatprocessor.DataMPEG4Audio{
			PTS: pts,
			AUs: [][]byte{au},
			NTP: time.Now(),
		})
		if err != nil {
			s.Log(logger.Warn, "%v", err)
		}
	}

	c, err := hls.NewClient(
		s.ur,
		s.fingerprint,
		onTracks,
		onVideoData,
		onAudioData,
		s,
	)
	if err != nil {
		return err
	}

	select {
	case err := <-c.Wait():
		return err

	case <-ctx.Done():
		c.Close()
		<-c.Wait()
		return nil
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*hlsSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsSource"}
}
