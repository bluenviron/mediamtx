package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/ringbuffer"
	"github.com/google/uuid"
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/mediamtx/internal/formatprocessor"
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
	iceServers        []string
	wg                *sync.WaitGroup
	pathManager       webRTCConnPathManager
	parent            webRTCConnParent
	iceUDPMux         ice.UDPMux
	iceTCPMux         ice.TCPMux
	iceHostNAT1To1IPs []string

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
	iceServers []string,
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
		iceServers:        iceServers,
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
		iceUDPMux:         iceUDPMux,
		iceTCPMux:         iceTCPMux,
		iceHostNAT1To1IPs: iceHostNAT1To1IPs,
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
	if c.publish {
		return c.runPublish(ctx)
	}
	return c.runRead(ctx)
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

	pc, err := newPeerConnection(
		c.videoCodec,
		c.audioCodec,
		c.genICEServers(),
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

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return err
	}

	err = pc.SetLocalDescription(offer)
	if err != nil {
		return err
	}

	tmp, err := strconv.ParseUint(c.videoBitrate, 10, 31)
	if err != nil {
		return err
	}

	insertTias(&offer, tmp*1024)

	err = c.writeOffer(&offer)
	if err != nil {
		return err
	}

	answer, err := c.readAnswer()
	if err != nil {
		return err
	}

	err = pc.SetRemoteDescription(*answer)
	if err != nil {
		return err
	}

	cr := newWebRTCCandidateReader(c.ws)
	defer cr.close()

	err = c.establishConnection(ctx, pc, cr)
	if err != nil {
		return err
	}

	close(cr.stopGathering)

	tracks, err := c.gatherIncomingTracks(ctx, pc, cr, trackRecv)
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

func (c *webRTCConn) runRead(ctx context.Context) error {
	res := c.pathManager.readerAdd(pathReaderAddReq{
		author:   c,
		pathName: c.pathName,
		skipAuth: true,
	})
	if res.err != nil {
		return res.err
	}

	defer res.path.readerRemove(pathReaderRemoveReq{author: c})

	tracks, err := c.gatherOutgoingTracks(res.stream.medias())
	if err != nil {
		return err
	}

	err = c.writeICEServers()
	if err != nil {
		return err
	}

	offer, err := c.readOffer()
	if err != nil {
		return err
	}

	pc, err := newPeerConnection(
		"",
		"",
		c.genICEServers(),
		c.iceHostNAT1To1IPs,
		c.iceUDPMux,
		c.iceTCPMux,
		c)
	if err != nil {
		return err
	}
	defer pc.close()

	for _, track := range tracks {
		var err error
		track.sender, err = pc.AddTrack(track.track)
		if err != nil {
			return err
		}
	}

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

	err = c.writeAnswer(&answer)
	if err != nil {
		return err
	}

	cr := newWebRTCCandidateReader(c.ws)
	defer cr.close()

	err = c.establishConnection(ctx, pc, cr)
	if err != nil {
		return err
	}

	close(cr.stopGathering)

	for _, track := range tracks {
		track.start()
	}

	ringBuffer, _ := ringbuffer.New(uint64(c.readBufferCount))
	defer ringBuffer.Close()

	writeError := make(chan error)

	for _, track := range tracks {
		ctrack := track
		res.stream.readerAdd(c, track.media, track.format, func(unit formatprocessor.Unit) {
			ringBuffer.Push(func() {
				ctrack.cb(unit, ctx, writeError)
			})
		})
	}
	defer res.stream.readerRemove(c)

	c.Log(logger.Info, "is reading from path '%s', %s",
		res.path.name, sourceMediaInfo(mediasOfOutgoingTracks(tracks)))

	go func() {
		for {
			item, ok := ringBuffer.Pull()
			if !ok {
				return
			}
			item.(func())()
		}
	}()

	select {
	case <-pc.disconnected:
		return fmt.Errorf("peer connection closed")

	case err := <-cr.readError:
		return fmt.Errorf("websocket error: %v", err)

	case err := <-writeError:
		return err

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (c *webRTCConn) gatherOutgoingTracks(medias media.Medias) ([]*webRTCOutgoingTrack, error) {
	var tracks []*webRTCOutgoingTrack

	videoTrack, err := newWebRTCOutgoingTrackVideo(medias)
	if err != nil {
		return nil, err
	}

	if videoTrack != nil {
		tracks = append(tracks, videoTrack)
	}

	audioTrack, err := newWebRTCOutgoingTrackAudio(medias)
	if err != nil {
		return nil, err
	}

	if audioTrack != nil {
		tracks = append(tracks, audioTrack)
	}

	if tracks == nil {
		return nil, fmt.Errorf(
			"the stream doesn't contain any supported codec, which are currently H264, VP8, VP9, G711, G722, Opus")
	}

	return tracks, nil
}

func (c *webRTCConn) gatherIncomingTracks(
	ctx context.Context,
	pc *peerConnection,
	cr *webRTCCandidateReader,
	trackRecv chan trackRecvPair,
) ([]*webRTCIncomingTrack, error) {
	var tracks []*webRTCIncomingTrack

	t := time.NewTimer(webrtcTrackGatherTimeout)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			return tracks, nil

		case pair := <-trackRecv:
			track, err := newWebRTCIncomingTrack(pair.track, pair.receiver, pc.WriteRTCP)
			if err != nil {
				return nil, err
			}
			tracks = append(tracks, track)

			if len(tracks) == 2 {
				return tracks, nil
			}

		case <-pc.disconnected:
			return nil, fmt.Errorf("peer connection closed")

		case err := <-cr.readError:
			return nil, fmt.Errorf("websocket error: %v", err)

		case <-ctx.Done():
			return nil, fmt.Errorf("terminated")
		}
	}
}

func (c *webRTCConn) genICEServers() []webrtc.ICEServer {
	ret := make([]webrtc.ICEServer, len(c.iceServers))
	for i, s := range c.iceServers {
		parts := strings.Split(s, ":")
		if len(parts) == 5 {
			if parts[1] == "AUTH_SECRET" {
				s := webrtc.ICEServer{
					URLs: []string{parts[0] + ":" + parts[3] + ":" + parts[4]},
				}

				randomUser := func() string {
					const charset = "abcdefghijklmnopqrstuvwxyz1234567890"
					b := make([]byte, 20)
					for i := range b {
						b[i] = charset[rand.Intn(len(charset))]
					}
					return string(b)
				}()

				expireDate := time.Now().Add(24 * 3600 * time.Second).Unix()
				s.Username = strconv.FormatInt(expireDate, 10) + ":" + randomUser

				h := hmac.New(sha1.New, []byte(parts[2]))
				h.Write([]byte(s.Username))
				s.Credential = base64.StdEncoding.EncodeToString(h.Sum(nil))

				ret[i] = s
			} else {
				ret[i] = webrtc.ICEServer{
					URLs:       []string{parts[0] + ":" + parts[3] + ":" + parts[4]},
					Username:   parts[1],
					Credential: parts[2],
				}
			}
		} else {
			ret[i] = webrtc.ICEServer{
				URLs: []string{s},
			}
		}
	}
	return ret
}

func (c *webRTCConn) establishConnection(
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

	c.Log(logger.Info, "peer connection established, local candidate: %v, remote candidate: %v",
		pc.localCandidate(), pc.remoteCandidate())

	return nil
}

func (c *webRTCConn) writeICEServers() error {
	return c.ws.WriteJSON(c.genICEServers())
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

func (c *webRTCConn) writeOffer(offer *webrtc.SessionDescription) error {
	return c.ws.WriteJSON(offer)
}

func (c *webRTCConn) readAnswer() (*webrtc.SessionDescription, error) {
	var answer webrtc.SessionDescription
	err := c.ws.ReadJSON(&answer)
	if err != nil {
		return nil, err
	}

	if answer.Type != webrtc.SDPTypeAnswer {
		return nil, fmt.Errorf("received SDP is not an offer")
	}

	return &answer, nil
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
