package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtph264"
	"github.com/aler9/gortsplib/v2/pkg/h264"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/ringbuffer"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type webRTCTrack struct {
	media       *media.Media
	format      format.Format
	webRTCTrack *webrtc.TrackLocalStaticRTP
	cb          func(data, context.Context, chan error)
}

func gatherMedias(tracks []*webRTCTrack) media.Medias {
	var ret media.Medias

	for _, track := range tracks {
		ret = append(ret, track.media)
	}

	return ret
}

type webRTCConnPathManager interface {
	readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes
}

type webRTCConnParent interface {
	log(logger.Level, string, ...interface{})
	connClose(*webRTCConn)
}

type webRTCConn struct {
	readBufferCount int
	pathName        string
	wsconn          *websocket.Conn
	stunServers     []string
	wg              *sync.WaitGroup
	pathManager     webRTCConnPathManager
	parent          webRTCConnParent

	ctx       context.Context
	ctxCancel func()
}

func newWebRTCConn(
	parentCtx context.Context,
	readBufferCount int,
	pathName string,
	wsconn *websocket.Conn,
	stunServers []string,
	wg *sync.WaitGroup,
	pathManager webRTCConnPathManager,
	parent webRTCConnParent,
) *webRTCConn {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	c := &webRTCConn{
		readBufferCount: readBufferCount,
		pathName:        pathName,
		wsconn:          wsconn,
		stunServers:     stunServers,
		wg:              wg,
		pathManager:     pathManager,
		parent:          parent,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
	}

	c.log(logger.Info, "opened")

	wg.Add(1)
	go c.run()

	return c
}

func (c *webRTCConn) close() {
	c.ctxCancel()
}

func (c *webRTCConn) log(level logger.Level, format string, args ...interface{}) {
	c.parent.log(level, "[conn %v] "+format, append([]interface{}{c.wsconn.RemoteAddr()}, args...)...)
}

func (c *webRTCConn) run() {
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

	c.log(logger.Info, "closed (%v)", err)
}

func (c *webRTCConn) runInner(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		c.wsconn.Close()
	}()

	res := c.pathManager.readerAdd(pathReaderAddReq{
		author:   c,
		pathName: c.pathName,
		authenticate: func(
			pathIPs []fmt.Stringer,
			pathUser conf.Credential,
			pathPass conf.Credential,
		) error {
			return nil
		},
	})
	if res.err != nil {
		return res.err
	}

	path := res.path

	defer func() {
		path.readerRemove(pathReaderRemoveReq{author: c})
	}()

	tracks, err := c.allocateTracks(res.stream.medias())
	if err != nil {
		return err
	}

	// maximum deadline to complete the handshake
	c.wsconn.SetReadDeadline(time.Now().Add(10 * time.Second))
	c.wsconn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	iceServers := c.iceServers()
	err = c.writeICEServers(iceServers)
	if err != nil {
		return err
	}

	offer, err := c.readOffer()
	if err != nil {
		return err
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	if err != nil {
		return err
	}
	defer pc.Close()

	for _, track := range tracks {
		_, err = pc.AddTrack(track.webRTCTrack)
		if err != nil {
			return err
		}
	}

	outgoingCandidate := make(chan *webrtc.ICECandidate)
	pcConnected := make(chan struct{})
	pcDisconnected := make(chan struct{})

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			select {
			case outgoingCandidate <- i:
			case <-pcConnected:
			case <-ctx.Done():
			}
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.log(logger.Debug, "peer connection state: "+state.String())

		switch state {
		case webrtc.PeerConnectionStateConnected:
			close(pcConnected)

		case webrtc.PeerConnectionStateDisconnected:
			close(pcDisconnected)
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

	err = c.writeAnswer(&answer)
	if err != nil {
		return err
	}

	readError := make(chan error)
	incomingCandidate := make(chan *webrtc.ICECandidateInit)

	go func() {
		for {
			candidate, err := c.readCandidate()
			if err != nil {
				select {
				case readError <- err:
				case <-pcConnected:
				case <-ctx.Done():
				}
				return
			}

			select {
			case incomingCandidate <- candidate:
			case <-pcConnected:
			case <-ctx.Done():
			}
		}
	}()

outer:
	for {
		select {
		case candidate := <-outgoingCandidate:
			c.writeCandidate(candidate)

		case candidate := <-incomingCandidate:
			err = pc.AddICECandidate(*candidate)
			if err != nil {
				return err
			}

		case err := <-readError:
			return err

		case <-pcConnected:
			break outer

		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	c.log(logger.Info, "peer connection established")
	c.wsconn.Close()

	ringBuffer, _ := ringbuffer.New(uint64(c.readBufferCount))
	defer ringBuffer.Close()

	writeError := make(chan error)

	for _, track := range tracks {
		res.stream.readerAdd(c, track.media, track.format, func(dat data) {
			ringBuffer.Push(func() {
				track.cb(dat, ctx, writeError)
			})
		})
	}
	defer res.stream.readerRemove(c)

	c.log(logger.Info, "is reading from path '%s', %s",
		path.Name(), sourceMediaInfo(gatherMedias(tracks)))

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
	case <-pcDisconnected:
		return fmt.Errorf("peer connection closed")

	case err := <-writeError:
		return err

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (c *webRTCConn) allocateTracks(medias media.Medias) ([]*webRTCTrack, error) {
	var ret []*webRTCTrack

	var videoFormat *format.H264
	videoMedia := medias.FindFormat(&videoFormat)

	if videoFormat != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: 90000,
			},
			"h264",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtph264.Encoder{
			PayloadType:    96,
			PayloadMaxSize: 1200,
		}
		encoder.Init()

		var lastPTS time.Duration
		firstNALUReceived := false

		ret = append(ret, &webRTCTrack{
			media:       videoMedia,
			format:      videoFormat,
			webRTCTrack: webRTCTrak,
			cb: func(dat data, ctx context.Context, writeError chan error) {
				tdata := dat.(*dataH264)

				if tdata.nalus == nil {
					return
				}

				if !firstNALUReceived {
					if !h264.IDRPresent(tdata.nalus) {
						return
					}

					firstNALUReceived = true
					lastPTS = tdata.pts
				} else {
					if tdata.pts < lastPTS {
						select {
						case writeError <- fmt.Errorf("WebRTC doesn't support H264 streams with B-frames"):
						case <-ctx.Done():
						}
						return
					}
					lastPTS = tdata.pts
				}

				packets, err := encoder.Encode(tdata.nalus, tdata.pts)
				if err != nil {
					return
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		})
	}

	var audioFormat *format.Opus
	audioMedia := medias.FindFormat(&audioFormat)

	if audioFormat != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: uint32(audioFormat.ClockRate()),
			},
			"opus",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		ret = append(ret, &webRTCTrack{
			media:       audioMedia,
			format:      audioFormat,
			webRTCTrack: webRTCTrak,
			cb: func(dat data, ctx context.Context, writeError chan error) {
				tdata := dat.(*dataOpus)

				for _, pkt := range tdata.rtpPackets {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		})
	}

	if ret == nil {
		return nil, fmt.Errorf("the stream doesn't contain any supported codec (which currently are H264 and Opus)")
	}

	return ret, nil
}

func (c *webRTCConn) iceServers() []webrtc.ICEServer {
	ret := make([]webrtc.ICEServer, len(c.stunServers))
	for i, s := range c.stunServers {
		ret[i] = webrtc.ICEServer{
			URLs: []string{"stun:" + s},
		}
	}
	return ret
}

func (c *webRTCConn) writeICEServers(iceServers []webrtc.ICEServer) error {
	enc, _ := json.Marshal(iceServers)
	return c.wsconn.WriteMessage(websocket.TextMessage, enc)
}

func (c *webRTCConn) readOffer() (*webrtc.SessionDescription, error) {
	_, enc, err := c.wsconn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var offer webrtc.SessionDescription
	err = json.Unmarshal(enc, &offer)
	if err != nil {
		return nil, err
	}

	if offer.Type != webrtc.SDPTypeOffer {
		return nil, fmt.Errorf("received SDP is not an offer")
	}

	return &offer, nil
}

func (c *webRTCConn) writeAnswer(answer *webrtc.SessionDescription) error {
	enc, _ := json.Marshal(answer)
	return c.wsconn.WriteMessage(websocket.TextMessage, enc)
}

func (c *webRTCConn) writeCandidate(candidate *webrtc.ICECandidate) error {
	enc, _ := json.Marshal(candidate.ToJSON())
	return c.wsconn.WriteMessage(websocket.TextMessage, enc)
}

func (c *webRTCConn) readCandidate() (*webrtc.ICECandidateInit, error) {
	_, enc, err := c.wsconn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var candidate webrtc.ICECandidateInit
	err = json.Unmarshal(enc, &candidate)
	if err != nil {
		return nil, err
	}

	return &candidate, err
}

// apiReaderDescribe implements reader.
func (c *webRTCConn) apiReaderDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"webRTCConn"}
}
