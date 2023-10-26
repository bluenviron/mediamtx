package core

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
)

type webRTCSourceParent interface {
	logger.Writer
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
}

type webRTCSource struct {
	readTimeout conf.StringDuration

	parent webRTCSourceParent
}

func newWebRTCSource(
	readTimeout conf.StringDuration,
	parent webRTCSourceParent,
) *webRTCSource {
	s := &webRTCSource{
		readTimeout: readTimeout,
		parent:      parent,
	}

	return s
}

func (s *webRTCSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[WebRTC source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *webRTCSource) run(ctx context.Context, cnf *conf.Path, _ chan *conf.Path) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(cnf.Source)
	if err != nil {
		return err
	}

	u.Scheme = strings.ReplaceAll(u.Scheme, "whep", "http")

	hc := &http.Client{
		Timeout: time.Duration(s.readTimeout),
	}

	client := webrtc.WHIPClient{
		HTTPClient: hc,
		URL:        u,
		Log:        s,
	}

	tracks, err := client.Read(ctx)
	if err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	medias := webrtcMediasOfIncomingTracks(tracks)

	rres := s.parent.setReady(pathSourceStaticSetReadyReq{
		desc:               &description.Session{Medias: medias},
		generateRTPPackets: true,
	})
	if rres.err != nil {
		return rres.err
	}

	defer s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})

	timeDecoder := rtptime.NewGlobalDecoder()

	for i, media := range medias {
		ci := i
		cmedia := media
		trackWrapper := &webrtcTrackWrapper{clockRate: cmedia.Formats[0].ClockRate()}

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

				rres.stream.WriteRTPPacket(cmedia, cmedia.Formats[0], pkt, time.Now(), pts)
			}
		}()
	}

	return client.Wait(ctx)
}

// apiSourceDescribe implements sourceStaticImpl.
func (*webRTCSource) apiSourceDescribe() apiPathSourceOrReader {
	return apiPathSourceOrReader{
		Type: "webRTCSource",
		ID:   "",
	}
}
