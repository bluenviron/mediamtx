package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/webrtcpc"
	"github.com/bluenviron/mediamtx/internal/whip"
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
func (s *webRTCSource) run(ctx context.Context, cnf *conf.PathConf, _ chan *conf.PathConf) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(cnf.Source)
	if err != nil {
		return err
	}

	u.Scheme = strings.ReplaceAll(u.Scheme, "whep", "http")

	c := &http.Client{
		Timeout: time.Duration(s.readTimeout),
	}

	iceServers, err := whip.GetICEServers(ctx, c, u.String())
	if err != nil {
		return err
	}

	api, err := webrtcNewAPI(nil, nil, nil)
	if err != nil {
		return err
	}

	pc, err := webrtcpc.New(iceServers, api, s)
	if err != nil {
		return err
	}
	defer pc.Close()

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		return err
	}

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	if err != nil {
		return err
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return err
	}

	err = pc.SetLocalDescription(offer)
	if err != nil {
		return err
	}

	err = pc.WaitGatheringDone(ctx)
	if err != nil {
		return err
	}

	res, err := whip.PostOffer(ctx, c, u.String(), pc.LocalDescription())
	if err != nil {
		return err
	}

	var sdp sdp.SessionDescription
	err = sdp.Unmarshal([]byte(res.Answer.SDP))
	if err != nil {
		return err
	}

	// check that there are at most two tracks
	_, err = webrtcTrackCount(sdp.MediaDescriptions)
	if err != nil {
		return err
	}

	trackRecv := make(chan trackRecvPair)

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		select {
		case trackRecv <- trackRecvPair{track, receiver}:
		case <-ctx.Done():
		}
	})

	err = pc.SetRemoteDescription(*res.Answer)
	if err != nil {
		return err
	}

	err = webrtcWaitUntilConnected(ctx, pc)
	if err != nil {
		return err
	}

	tracks, err := webrtcGatherIncomingTracks(ctx, pc, trackRecv, 0)
	if err != nil {
		return err
	}
	medias := webrtcMediasOfIncomingTracks(tracks)

	rres := s.parent.setReady(pathSourceStaticSetReadyReq{
		medias:             medias,
		generateRTPPackets: true,
	})
	if rres.err != nil {
		return rres.err
	}

	defer s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})

	for _, track := range tracks {
		track.start(rres.stream)
	}

	select {
	case <-pc.Disconnected():
		return fmt.Errorf("peer connection closed")

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*webRTCSource) apiSourceDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "webRTCSource",
		ID:   "",
	}
}
