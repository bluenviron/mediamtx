package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
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
func (s *rtmpSource) run(ctx context.Context, cnf *conf.Path, reloadConf chan *conf.Path) error {
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

	var medias []*description.Media
	var stream *stream.Stream

	if videoFormat != nil {
		videoMedia := &description.Media{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{videoFormat},
		}
		medias = append(medias, videoMedia)

		switch videoFormat.(type) {
		case *format.H264:
			mc.OnDataH264(func(pts time.Duration, au [][]byte) {
				stream.WriteUnit(videoMedia, videoFormat, &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AU: au,
				})
			})

		default:
			return fmt.Errorf("unsupported video codec: %T", videoFormat)
		}
	}

	if audioFormat != nil { //nolint:dupl
		audioMedia := &description.Media{
			Type:    description.MediaTypeAudio,
			Formats: []format.Format{audioFormat},
		}
		medias = append(medias, audioMedia)

		switch audioFormat.(type) {
		case *format.MPEG4Audio:
			mc.OnDataMPEG4Audio(func(pts time.Duration, au []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AUs: [][]byte{au},
				})
			})

		case *format.MPEG1Audio:
			mc.OnDataMPEG1Audio(func(pts time.Duration, frame []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Frames: [][]byte{frame},
				})
			})

		default:
			return fmt.Errorf("unsupported audio codec: %T", audioFormat)
		}
	}

	res := s.parent.setReady(pathSourceStaticSetReadyReq{
		desc:               &description.Session{Medias: medias},
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
func (*rtmpSource) apiSourceDescribe() apiPathSourceOrReader {
	return apiPathSourceOrReader{
		Type: "rtmpSource",
		ID:   "",
	}
}
