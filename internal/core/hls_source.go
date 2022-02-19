package core

import (
	"context"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"

	"github.com/aler9/rtsp-simple-server/internal/hls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtcpsenderset"
)

const (
	hlsSourceRetryPause = 5 * time.Second
)

type hlsSourceParent interface {
	log(logger.Level, string, ...interface{})
	onSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	onSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type hlsSource struct {
	ur          string
	fingerprint string
	wg          *sync.WaitGroup
	parent      hlsSourceParent

	ctx       context.Context
	ctxCancel func()
}

func newHLSSource(
	parentCtx context.Context,
	ur string,
	fingerprint string,
	wg *sync.WaitGroup,
	parent hlsSourceParent) *hlsSource {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &hlsSource{
		ur:          ur,
		fingerprint: fingerprint,
		wg:          wg,
		parent:      parent,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
	}

	s.Log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()

	return s
}

func (s *hlsSource) close() {
	s.Log(logger.Info, "stopped")
	s.ctxCancel()
}

func (s *hlsSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[hls source] "+format, args...)
}

func (s *hlsSource) run() {
	defer s.wg.Done()

outer:
	for {
		ok := s.runInner()
		if !ok {
			break outer
		}

		select {
		case <-time.After(hlsSourceRetryPause):
		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()
}

func (s *hlsSource) runInner() bool {
	var stream *stream
	var rtcpSenders *rtcpsenderset.RTCPSenderSet
	var videoTrackID int
	var audioTrackID int
	var videoEnc *rtph264.Encoder
	var audioEnc *rtpaac.Encoder

	defer func() {
		if stream != nil {
			s.parent.onSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{source: s})
			rtcpSenders.Close()
		}
	}()

	onTracks := func(videoTrack gortsplib.Track, audioTrack gortsplib.Track) error {
		var tracks gortsplib.Tracks

		if videoTrack != nil {
			videoTrackID = len(tracks)
			videoEnc = rtph264.NewEncoder(96, nil, nil, nil)
			tracks = append(tracks, videoTrack)
		}

		if audioTrack != nil {
			audioTrackID = len(tracks)
			audioEnc = rtpaac.NewEncoder(97, audioTrack.ClockRate(), nil, nil, nil)
			tracks = append(tracks, audioTrack)
		}

		res := s.parent.onSourceStaticSetReady(pathSourceStaticSetReadyReq{
			source: s,
			tracks: tracks,
		})
		if res.err != nil {
			return res.err
		}

		s.Log(logger.Info, "ready")

		stream = res.stream
		rtcpSenders = rtcpsenderset.New(tracks, stream.onPacketRTCP)

		return nil
	}

	onVideoData := func(pts time.Duration, nalus [][]byte) {
		if stream == nil {
			return
		}

		pkts, err := videoEnc.Encode(nalus, pts)
		if err != nil {
			return
		}

		for _, pkt := range pkts {
			rtcpSenders.OnPacketRTP(videoTrackID, pkt)
			stream.onPacketRTP(videoTrackID, pkt)
		}
	}

	onAudioData := func(pts time.Duration, aus [][]byte) {
		if stream == nil {
			return
		}

		pkts, err := audioEnc.Encode(aus, pts)
		if err != nil {
			return
		}

		for _, pkt := range pkts {
			rtcpSenders.OnPacketRTP(audioTrackID, pkt)
			stream.onPacketRTP(audioTrackID, pkt)
		}
	}

	c, err := hls.NewClient(
		s.ur,
		s.fingerprint,
		onTracks,
		onVideoData,
		onAudioData,
		s,
	)
	if err != nil {
		s.Log(logger.Info, "ERR: %v", err)
		return true
	}

	select {
	case err := <-c.Wait():
		s.Log(logger.Info, "ERR: %v", err)
		return true

	case <-s.ctx.Done():
		c.Close()
		<-c.Wait()
		return false
	}
}

// onSourceAPIDescribe implements source.
func (*hlsSource) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"hlsSource"}
}
