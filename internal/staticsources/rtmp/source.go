// Package rtmp contains the RTMP static source.
package rtmp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a RTMP static source.
type Source struct {
	ReadTimeout  conf.Duration
	WriteTimeout conf.Duration
	Parent       parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[RTMP source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	// add default port
	_, _, err = net.SplitHostPort(u.Host)
	if err != nil {
		if u.Scheme == "rtmp" {
			u.Host = net.JoinHostPort(u.Host, "1935")
		} else {
			u.Host = net.JoinHostPort(u.Host, "1936")
		}
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	readDone := make(chan error)
	go func() {
		readDone <- s.runReader(ctx, u, params.Conf.SourceFingerprint)
	}()

	for {
		select {
		case err = <-readDone:
			ctxCancel()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			ctxCancel()
			<-readDone
			return nil
		}
	}
}

func (s *Source) runReader(ctx context.Context, u *url.URL, fingerprint string) error {
	connectCtx, connectCtxCancel := context.WithTimeout(ctx, time.Duration(s.ReadTimeout))
	conn := &gortmplib.Client{
		URL:       u,
		TLSConfig: tls.MakeConfig(u.Hostname(), fingerprint),
		Publish:   false,
	}
	err := conn.Initialize(connectCtx)
	connectCtxCancel()
	if err != nil {
		return err
	}

	r := &gortmplib.Reader{
		Conn: conn,
	}
	err = r.Initialize()
	if err != nil {
		conn.Close()
		return err
	}

	var stream *stream.Stream

	medias, err := rtmp.ToStream(r, &stream)
	if err != nil {
		conn.Close()
		return err
	}

	if len(medias) == 0 {
		conn.Close()
		return fmt.Errorf("no supported tracks found")
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
		FillNTP:            true,
	})
	if res.Err != nil {
		conn.Close()
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	stream = res.Stream

	readerErr := make(chan error)
	go func() {
		for {
			conn.NetConn().SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
			err2 := r.Read()
			if err2 != nil {
				readerErr <- err2
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		conn.Close()
		<-readerErr
		return fmt.Errorf("terminated")

	case err = <-readerErr:
		return err
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "rtmpSource",
		ID:   "",
	}
}
