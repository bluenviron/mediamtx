// Package webrtc contains the WebRTC static source.
package webrtc

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a WebRTC static source.
type Source struct {
	ReadTimeout       conf.Duration
	UDPReadBufferSize uint
	Parent            parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[WebRTC source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	u.Scheme = strings.ReplaceAll(u.Scheme, "whep", "http")

	tr := &http.Transport{
		TLSClientConfig: tls.MakeConfig(u.Hostname(), params.Conf.SourceFingerprint),
	}
	defer tr.CloseIdleConnections()

	client := whip.Client{
		URL: u,
		HTTPClient: &http.Client{
			Timeout:   time.Duration(s.ReadTimeout),
			Transport: tr,
		},
		UDPReadBufferSize: s.UDPReadBufferSize,
		Log:               s,
	}
	err = client.Initialize(params.Context)
	if err != nil {
		return err
	}

	var stream *stream.Stream

	medias, err := webrtc.ToStream(client.PeerConnection(), params.Conf, &stream, s)
	if err != nil {
		client.Close() //nolint:errcheck
		return err
	}

	rres := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
		FillNTP:            !params.Conf.UseAbsoluteTimestamp,
	})
	if rres.Err != nil {
		client.Close() //nolint:errcheck
		return rres.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	stream = rres.Stream

	client.StartReading()

	readErr := make(chan error)

	go func() {
		readErr <- client.Wait()
	}()

	for {
		select {
		case err = <-readErr:
			client.Close() //nolint:errcheck
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			client.Close() //nolint:errcheck
			<-readErr
			return fmt.Errorf("terminated")
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "webRTCSource",
		ID:   "",
	}
}
