// Package webrtc contains the WebRTC static source.
package webrtc

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
)

// Source is a WebRTC static source.
type Source struct {
	ReadTimeout conf.StringDuration
	Parent      defs.StaticSourceParent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
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
		TLSClientConfig: tls.ConfigForFingerprint(params.Conf.SourceFingerprint),
	}
	defer tr.CloseIdleConnections()

	client := webrtc.WHIPClient{
		HTTPClient: &http.Client{
			Timeout:   time.Duration(s.ReadTimeout),
			Transport: tr,
		},
		URL: u,
		Log: s,
	}

	tracks, err := client.Read(params.Context)
	if err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	medias := webrtc.TracksToMedias(tracks)

	rres := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
	})
	if rres.Err != nil {
		return rres.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	timeDecoder := rtptime.NewGlobalDecoder()

	for i, media := range medias {
		ci := i
		cmedia := media
		trackWrapper := &webrtc.TrackWrapper{ClockRat: cmedia.Formats[0].ClockRate()}

		go func() {
			for {
				pkt, err := tracks[ci].ReadRTP()
				if err != nil {
					return
				}

				pts, ok := timeDecoder.Decode(trackWrapper, pkt)
				if !ok {
					continue
				}

				rres.Stream.WriteRTPPacket(cmedia, cmedia.Formats[0], pkt, time.Now(), pts)
			}
		}()
	}

	return client.Wait(params.Context)
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "webrtcSource",
		ID:   "",
	}
}
