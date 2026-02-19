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
	"github.com/bluenviron/mediamtx/internal/packetdumper"
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
	DumpPackets  bool
	ReadTimeout  conf.Duration
	WriteTimeout conf.Duration
	Parent       parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
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

	dialContext := (&net.Dialer{}).DialContext

	if s.DumpPackets {
		dialContext = (&packetdumper.DialContext{
			Prefix:      "rtmp_source_conn",
			DialContext: dialContext,
		}).Do
	}

	connectCtx, connectCtxCancel := context.WithTimeout(params.Context, time.Duration(s.ReadTimeout))

	conn := &gortmplib.Client{
		URL:         u,
		TLSConfig:   tls.MakeConfig(u.Hostname(), params.Conf.SourceFingerprint),
		Publish:     false,
		DialContext: dialContext,
	}
	err = conn.Initialize(connectCtx)
	connectCtxCancel()
	if err != nil {
		return err
	}

	readDone := make(chan error)
	go func() {
		readDone <- s.runReader(conn)
	}()

	for {
		select {
		case err = <-readDone:
			conn.Close()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			conn.Close()
			<-readDone
			return nil
		}
	}
}

func (s *Source) runReader(conn *gortmplib.Client) error {
	conn.NetConn().SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
	conn.NetConn().SetWriteDeadline(time.Now().Add(time.Duration(s.WriteTimeout)))

	r := &gortmplib.Reader{
		Conn: conn,
	}
	err := r.Initialize()
	if err != nil {
		return err
	}

	var subStream *stream.SubStream

	medias, err := rtmp.ToStream(r, &subStream)
	if err != nil {
		return err
	}

	if len(medias) == 0 {
		return fmt.Errorf("no supported tracks found")
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: false,
		ReplaceNTP:    true,
	})
	if res.Err != nil {
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = res.SubStream

	conn.NetConn().SetWriteDeadline(time.Time{})

	for {
		conn.NetConn().SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err = r.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: "rtmpSource",
		ID:   "",
	}
}
