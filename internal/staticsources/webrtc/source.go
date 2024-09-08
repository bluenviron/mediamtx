// Package webrtc contains the WebRTC static source.
package webrtc

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"
	"github.com/pion/rtp"

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

	for i, track := range tracks {
		medi := medias[i]
		ctrack := tracks[i]

		track.OnPacketRTP = func(pkt *rtp.Packet) {
			pts, ok := timeDecoder.Decode(ctrack, pkt)
			if !ok {
				return
			}

			rres.Stream.WriteRTPPacket(medi, medi.Formats[0], pkt, time.Now(), pts)
		}
	}

	client.StartReading()

	return client.Wait(params.Context)
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "webrtcSource",
		ID:   "",
	}
}
