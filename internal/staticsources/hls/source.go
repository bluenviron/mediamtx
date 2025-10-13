// Package hls contains the HLS static source.
package hls

import (
	"net/http"
	"net/url"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a HLS static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[HLS source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	var stream *stream.Stream

	defer func() {
		if stream != nil {
			s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})
		}
	}()

	decodeErrors := &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Log(logger.Warn, "%d decode %s",
				val,
				func() string {
					if val == 1 {
						return "error"
					}
					return "errors"
				}())
		},
	}

	decodeErrors.Start()
	defer decodeErrors.Stop()

	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	tr := &http.Transport{
		TLSClientConfig: tls.MakeConfig(u.Hostname(), params.Conf.SourceFingerprint),
	}
	defer tr.CloseIdleConnections()

	var c *gohlslib.Client
	c = &gohlslib.Client{
		URI: params.ResolvedSource,
		HTTPClient: &http.Client{
			Timeout:   time.Duration(s.ReadTimeout),
			Transport: tr,
		},
		OnDownloadPrimaryPlaylist: func(u string) {
			s.Log(logger.Debug, "downloading primary playlist %v", u)
		},
		OnDownloadStreamPlaylist: func(u string) {
			s.Log(logger.Debug, "downloading stream playlist %v", u)
		},
		OnDownloadSegment: func(u string) {
			s.Log(logger.Debug, "downloading segment %v", u)
		},
		OnDownloadPart: func(u string) {
			s.Log(logger.Debug, "downloading part %v", u)
		},
		OnDecodeError: func(_ error) {
			decodeErrors.Increase()
		},
		OnTracks: func(tracks []*gohlslib.Track) error {
			medias, err2 := hls.ToStream(c, tracks, params.Conf, &stream)
			if err2 != nil {
				return err2
			}

			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:               &description.Session{Medias: medias},
				GenerateRTPPackets: true,
			})
			if res.Err != nil {
				return res.Err
			}

			stream = res.Stream

			return nil
		},
	}

	err = c.Start()
	if err != nil {
		return err
	}

	waitErr := make(chan error)
	go func() {
		waitErr <- c.Wait2()
	}()

	for {
		select {
		case err = <-waitErr:
			c.Close()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			c.Close()
			<-waitErr
			return nil
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "hlsSource",
		ID:   "",
	}
}
