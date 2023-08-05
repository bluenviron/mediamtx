package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type rtmpSourceParent interface {
	logger.Writer
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
}

type rtmpSource struct {
	readTimeout  conf.StringDuration
	writeTimeout conf.StringDuration
	parent       rtmpSourceParent
}

func newRTMPSource(
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	parent rtmpSourceParent,
) *rtmpSource {
	return &rtmpSource{
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		parent:       parent,
	}
}

func (s *rtmpSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[RTMP source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *rtmpSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(cnf.Source)
	if err != nil {
		return err
	}

	// add default port
	_, _, err = net.SplitHostPort(u.Host)
	if err != nil {
		u.Host = net.JoinHostPort(u.Host, "1935")
	}

	nconn, err := func() (net.Conn, error) {
		ctx2, cancel2 := context.WithTimeout(ctx, time.Duration(s.readTimeout))
		defer cancel2()

		if u.Scheme == "rtmp" {
			return (&net.Dialer{}).DialContext(ctx2, "tcp", u.Host)
		}

		return (&tls.Dialer{
			Config: tlsConfigForFingerprint(cnf.SourceFingerprint),
		}).DialContext(ctx2, "tcp", u.Host)
	}()
	if err != nil {
		return err
	}

	readDone := make(chan error)
	go func() {
		readDone <- s.runReader(u, nconn)
	}()

	for {
		select {
		case err := <-readDone:
			nconn.Close()
			return err

		case <-reloadConf:

		case <-ctx.Done():
			nconn.Close()
			<-readDone
			return nil
		}
	}
}

func (s *rtmpSource) runReader(u *url.URL, nconn net.Conn) error {
	nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
	nconn.SetWriteDeadline(time.Now().Add(time.Duration(s.writeTimeout)))
	conn, err := rtmp.NewClientConn(nconn, u, false)
	if err != nil {
		return err
	}

	mc, err := rtmp.NewReader(conn)
	if err != nil {
		return err
	}

	videoFormat, audioFormat := mc.Tracks()

	switch videoFormat.(type) {
	case *formats.H265, *formats.AV1:
		return fmt.Errorf("proxying H265 or AV1 tracks with RTMP is not supported")
	}

	var medias media.Medias
	var stream *stream.Stream

	if videoFormat != nil {
		videoMedia := &media.Media{
			Type:    media.TypeVideo,
			Formats: []formats.Format{videoFormat},
		}
		medias = append(medias, videoMedia)

		if _, ok := videoFormat.(*formats.H264); ok {
			mc.OnDataH264(func(pts time.Duration, au [][]byte) {
				stream.WriteUnit(videoMedia, videoFormat, &formatprocessor.UnitH264{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: pts,
					AU:  au,
				})
			})
		}
	}

	if audioFormat != nil { //nolint:dupl
		audioMedia := &media.Media{
			Type:    media.TypeAudio,
			Formats: []formats.Format{audioFormat},
		}
		medias = append(medias, audioMedia)

		switch audioFormat.(type) {
		case *formats.MPEG4AudioGeneric:
			mc.OnDataMPEG4Audio(func(pts time.Duration, au []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &formatprocessor.UnitMPEG4AudioGeneric{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: pts,
					AUs: [][]byte{au},
				})
			})

		case *formats.MPEG1Audio:
			mc.OnDataMPEG1Audio(func(pts time.Duration, frame []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &formatprocessor.UnitMPEG1Audio{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS:    pts,
					Frames: [][]byte{frame},
				})
			})
		}
	}

	res := s.parent.setReady(pathSourceStaticSetReadyReq{
		medias:             medias,
		generateRTPPackets: true,
	})
	if res.err != nil {
		return res.err
	}

	defer s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})

	stream = res.stream

	// disable write deadline to allow outgoing acknowledges
	nconn.SetWriteDeadline(time.Time{})

	for {
		nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
		err := mc.Read()
		if err != nil {
			return err
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*rtmpSource) apiSourceDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "rtmpSource",
		ID:   "",
	}
}
