package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/google/uuid"
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/mediamtx/internal/logger"
	"github.com/aler9/mediamtx/internal/websocket"
)

const (
	webrtcHandshakeTimeout   = 10 * time.Second
	webrtcTrackGatherTimeout = 2 * time.Second
	webrtcPayloadMaxSize     = 1188 // 1200 - 12 (RTP header)
)

type trackRecvPair struct {
	track    *webrtc.TrackRemote
	receiver *webrtc.RTPReceiver
}

func mediasOfOutgoingTracks(tracks []*webRTCOutgoingTrack) media.Medias {
	ret := make(media.Medias, len(tracks))
	for i, track := range tracks {
		ret[i] = track.media
	}
	return ret
}

func mediasOfIncomingTracks(tracks []*webRTCIncomingTrack) media.Medias {
	ret := make(media.Medias, len(tracks))
	for i, track := range tracks {
		ret[i] = track.media
	}
	return ret
}

func insertTias(offer *webrtc.SessionDescription, value uint64) {
	var sd sdp.SessionDescription
	err := sd.Unmarshal([]byte(offer.SDP))
	if err != nil {
		return
	}

	for _, media := range sd.MediaDescriptions {
		if media.MediaName.Media == "video" {
			media.Bandwidth = append(media.Bandwidth, sdp.Bandwidth{
				Type:      "TIAS",
				Bandwidth: value,
			})
		}
	}

	enc, err := sd.Marshal()
	if err != nil {
		return
	}

	offer.SDP = string(enc)
}

type webRTCConnPathManager interface {
	publisherAdd(req pathPublisherAddReq) pathPublisherAnnounceRes
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
}

type webRTCConnParent interface {
	logger.Writer
	genICEServers() []webrtc.ICEServer
	connClose(*webRTCConn)
}

type webRTCConn struct {
	readBufferCount   int
	pathName          string
	publish           bool
	ws                *websocket.ServerConn
	videoCodec        string
	audioCodec        string
	videoBitrate      string
	wg                *sync.WaitGroup
	pathManager       webRTCConnPathManager
	parent            webRTCConnParent
	iceHostNAT1To1IPs []string
	iceUDPMux         ice.UDPMux
	iceTCPMux         ice.TCPMux

	ctx       context.Context
	ctxCancel func()
	uuid      uuid.UUID
	created   time.Time
	pc        *peerConnection
	mutex     sync.RWMutex

	closed chan struct{}
}

func newWebRTCConn(
	parentCtx context.Context,
	readBufferCount int,
	pathName string,
	publish bool,
	ws *websocket.ServerConn,
	videoCodec string,
	audioCodec string,
	videoBitrate string,
	wg *sync.WaitGroup,
	pathManager webRTCConnPathManager,
	parent webRTCConnParent,
	iceHostNAT1To1IPs []string,
	iceUDPMux ice.UDPMux,
	iceTCPMux ice.TCPMux,
) *webRTCConn {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	c := &webRTCConn{
		readBufferCount:   readBufferCount,
		pathName:          pathName,
		publish:           publish,
		ws:                ws,
		wg:                wg,
		videoCodec:        videoCodec,
		audioCodec:        audioCodec,
		videoBitrate:      videoBitrate,
		pathManager:       pathManager,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		uuid:              uuid.New(),
		created:           time.Now(),
		iceHostNAT1To1IPs: iceHostNAT1To1IPs,
		iceUDPMux:         iceUDPMux,
		iceTCPMux:         iceTCPMux,
		closed:            make(chan struct{}),
	}

	c.Log(logger.Info, "opened")

	wg.Add(1)
	go c.run()

	return c
}

func (c *webRTCConn) close() {
	c.ctxCancel()
}

func (c *webRTCConn) wait() {
	<-c.closed
}

func (c *webRTCConn) remoteAddr() net.Addr {
	return c.ws.RemoteAddr()
}

func (c *webRTCConn) safePC() *peerConnection {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.pc
}

func (c *webRTCConn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.ws.RemoteAddr()}, args...)...)
}

func (c *webRTCConn) run() {
	defer close(c.closed)
	defer c.wg.Done()

	innerCtx, innerCtxCancel := context.WithCancel(c.ctx)
	runErr := make(chan error)
	go func() {
		runErr <- c.runInner(innerCtx)
	}()

	var err error
	select {
	case err = <-runErr:
		innerCtxCancel()

	case <-c.ctx.Done():
		innerCtxCancel()
		<-runErr
		err = errors.New("terminated")
	}

	c.ctxCancel()

	c.parent.connClose(c)

	c.Log(logger.Info, "closed (%v)", err)
}

func (c *webRTCConn) runInner(ctx context.Context) error {
	return c.runPublish(ctx)
}

func (c *webRTCConn) runPublish(ctx context.Context) error {
	res := c.pathManager.publisherAdd(pathPublisherAddReq{
		author:   c,
		pathName: c.pathName,
		skipAuth: true,
	})
	if res.err != nil {
		return res.err
	}

	defer res.path.publisherRemove(pathPublisherRemoveReq{author: c})

	err := c.writeICEServers()
	if err != nil {
		return err
	}

	offer, err := c.readOffer()
	if err != nil {
		return err
	}

	pc, err := newPeerConnection(
		c.videoCodec,
		c.audioCodec,
		c.parent.genICEServers(),
		c.iceHostNAT1To1IPs,
		c.iceUDPMux,
		c.iceTCPMux,
		c)
	if err != nil {
		return err
	}
	defer pc.close()

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RtpTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return err
	}

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RtpTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return err
	}

	trackRecv := make(chan trackRecvPair)

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		select {
		case trackRecv <- trackRecvPair{track, receiver}:
		case <-pc.closed:
		}
	})

	err = pc.SetRemoteDescription(*offer)
	if err != nil {
		return err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}

	err = pc.SetLocalDescription(answer)
	if err != nil {
		return err
	}

	tmp, err := strconv.ParseUint(c.videoBitrate, 10, 31)
	if err != nil {
		return err
	}

	insertTias(&answer, tmp*1024)

	err = c.writeAnswer(&answer)
	if err != nil {
		return err
	}

	cr := newWebRTCCandidateReader(c.ws)
	defer cr.close()

	err = c.waitUntilConnected(ctx, pc, cr)
	if err != nil {
		return err
	}

	close(cr.stopGathering)

	tracks, err := gatherIncomingTracks(ctx, pc, cr, trackRecv)
	if err != nil {
		return err
	}
	medias := mediasOfIncomingTracks(tracks)

	rres := res.path.publisherStart(pathPublisherStartReq{
		author:             c,
		medias:             medias,
		generateRTPPackets: false,
	})
	if rres.err != nil {
		return rres.err
	}

	c.Log(logger.Info, "is publishing to path '%s', %s",
		res.path.name,
		sourceMediaInfo(medias))

	for _, track := range tracks {
		track.start(rres.stream)
	}

	select {
	case <-pc.disconnected:
		return fmt.Errorf("peer connection closed")

	case err := <-cr.readError:
		return fmt.Errorf("websocket error: %v", err)

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (c *webRTCConn) waitUntilConnected(
	ctx context.Context,
	pc *peerConnection,
	cr *webRTCCandidateReader,
) error {
	t := time.NewTimer(webrtcHandshakeTimeout)
	defer t.Stop()

outer:
	for {
		select {
		case candidate := <-pc.localCandidateRecv:
			c.Log(logger.Debug, "local candidate: %+v", candidate.Candidate)
			err := c.ws.WriteJSON(candidate)
			if err != nil {
				return err
			}

		case candidate := <-cr.remoteCandidate:
			c.Log(logger.Debug, "remote candidate: %+v", candidate.Candidate)
			err := pc.AddICECandidate(*candidate)
			if err != nil {
				return err
			}

		case err := <-cr.readError:
			return err

		case <-t.C:
			return fmt.Errorf("deadline exceeded")

		case <-pc.connected:
			break outer

		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	// Keep WebSocket connection open and use it to notify shutdowns.
	// This is because pion/webrtc doesn't write yet a WebRTC shutdown
	// message to clients (like a DTLS close alert or a RTCP BYE),
	// therefore browsers do not properly detect shutdowns and do not
	// attempt to restart the connection immediately.

	c.mutex.Lock()
	c.pc = pc
	c.mutex.Unlock()

	return nil
}

func (c *webRTCConn) writeICEServers() error {
	return c.ws.WriteJSON(c.parent.genICEServers())
}

func (c *webRTCConn) readOffer() (*webrtc.SessionDescription, error) {
	var offer webrtc.SessionDescription
	err := c.ws.ReadJSON(&offer)
	if err != nil {
		return nil, err
	}

	if offer.Type != webrtc.SDPTypeOffer {
		return nil, fmt.Errorf("received SDP is not an offer")
	}

	return &offer, nil
}

func (c *webRTCConn) writeAnswer(answer *webrtc.SessionDescription) error {
	return c.ws.WriteJSON(answer)
}

// apiSourceDescribe implements sourceStaticImpl.
func (c *webRTCConn) apiSourceDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "webRTCConn",
		ID:   c.uuid.String(),
	}
}

// apiReaderDescribe implements reader.
func (c *webRTCConn) apiReaderDescribe() pathAPISourceOrReader {
	return c.apiSourceDescribe()
}
