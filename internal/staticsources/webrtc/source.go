// Package webrtc contains the WebRTC static source.
package webrtc

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/packetdumper"
	ptls "github.com/bluenviron/mediamtx/internal/protocols/tls"
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
	DumpPackets        bool
	ReadTimeout        conf.Duration
	UDPReadBufferSize  uint
	UDPWriteBufferSize uint
	SupportsIPv6       bool
	Parent             parent

	mutex  sync.RWMutex
	client *whip.Client
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[WebRTC source] "+format, args...)
}

// Info returns runtime information.
func (s *Source) Info() defs.StaticSourceInfo {
	s.mutex.RLock()
	client := s.client
	s.mutex.RUnlock()

	if client == nil || client.PeerConnection() == nil {
		return defs.StaticSourceInfo{}
	}

	pc := client.PeerConnection()
	stats := pc.Stats()

	return defs.StaticSourceInfo{
		TypeSpecific: &defs.APIStaticSourceTypeSpecificWebRTC{
			RemoteAddr:                client.URL.Host,
			PeerConnectionEstablished: true,
			LocalCandidate:            pc.LocalCandidate(),
			RemoteCandidate:           pc.RemoteCandidate(),
			InboundBytes:              stats.BytesReceived,
			InboundRTPPackets:         stats.RTPPacketsReceived,
			InboundRTPPacketsLost:     stats.RTPPacketsLost,
			InboundRTPPacketsJitter:   stats.RTPPacketsJitter,
			InboundRTCPPackets:        stats.RTCPPacketsReceived,
			OutboundBytes:             stats.BytesSent,
			OutboundRTPPackets:        stats.RTPPacketsSent,
			OutboundRTCPPackets:       stats.RTCPPacketsSent,
		},
	}
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()

	tlsConfig := ptls.MakeConfig(params.Conf.SourceFingerprint)

	if s.DumpPackets {
		tr.DialContext = (&packetdumper.DialContext{
			Prefix: u.Scheme + "_source_conn",
		}).Do

		tr.DialTLSContext = (&packetdumper.DialTLSContext{
			DialContext: tr.DialContext,
			TLSConfig:   tlsConfig,
		}).Do
	} else {
		tr.TLSClientConfig = tlsConfig
	}

	u.Scheme = strings.Replace(u.Scheme, "whep", "http", 1)

	client := whip.Client{
		URL: u,
		HTTPClient: &http.Client{
			Timeout:   time.Duration(s.ReadTimeout),
			Transport: tr,
		},
		BearerToken:        params.Conf.WHEPBearerToken,
		UDPReadBufferSize:  s.UDPReadBufferSize,
		UDPWriteBufferSize: s.UDPWriteBufferSize,
		SupportsIPv6:       s.SupportsIPv6,
		STUNGatherTimeout:  time.Duration(params.Conf.WHEPSTUNGatherTimeout),
		HandshakeTimeout:   time.Duration(params.Conf.WHEPHandshakeTimeout),
		TrackGatherTimeout: time.Duration(params.Conf.WHEPTrackGatherTimeout),
		Log:                s,
	}
	err = client.Initialize(params.Context)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	s.client = &client
	s.mutex.Unlock()

	defer func() {
		s.mutex.Lock()
		s.client = nil
		s.mutex.Unlock()
	}()

	var subStream *stream.SubStream

	medias, err := webrtc.ToStream(client.PeerConnection(), params.Conf, &subStream, s)
	if err != nil {
		client.Close() //nolint:errcheck
		return err
	}

	rres := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          &description.Session{Medias: medias},
		UseRTPPackets: true,
		ReplaceNTP:    !params.Conf.UseAbsoluteTimestamp,
	})
	if rres.Err != nil {
		client.Close() //nolint:errcheck
		return rres.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = rres.SubStream

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
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: defs.APIPathSourceTypeWebRTCSource,
		ID:   "",
	}
}
