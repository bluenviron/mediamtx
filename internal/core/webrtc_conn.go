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

	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtph264"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpvp8"
	"github.com/aler9/gortsplib/v2/pkg/formatdecenc/rtpvp9"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/ringbuffer"
	"github.com/google/uuid"
	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/websocket"
)

const (
	webrtcHandshakeDeadline = 10 * time.Second
	webrtcWsWriteDeadline   = 2 * time.Second
	webrtcPayloadMaxSize    = 1188 // 1200 - 12 (RTP header)
)

// newPeerConnection creates a PeerConnection with the default codecs and
// interceptors.  See RegisterDefaultCodecs and RegisterDefaultInterceptors.
//
// This function is a copy of webrtc/peerconnection.go
// unlike the original one, allows you to add additional custom options
func newPeerConnection(configuration webrtc.Configuration,
	options ...func(*webrtc.API),
) (*webrtc.PeerConnection, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return nil, err
	}

	options = append(options, webrtc.WithMediaEngine(m))
	options = append(options, webrtc.WithInterceptorRegistry(i))

	api := webrtc.NewAPI(options...)
	return api.NewPeerConnection(configuration)
}

type webRTCTrack struct {
	media       *media.Media
	format      format.Format
	webRTCTrack *webrtc.TrackLocalStaticRTP
	cb          func(formatprocessor.Data, context.Context, chan error)
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
	readBufferCount   int
	pathName          string
	wsconn            *websocket.ServerConn
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
	curPC     *webrtc.PeerConnection
	mutex     sync.RWMutex

	closed chan struct{}
}

func newWebRTCConn(
	parentCtx context.Context,
	readBufferCount int,
	pathName string,
	wsconn *websocket.ServerConn,
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
		wsconn:            wsconn,
		iceServers:        iceServers,
		wg:                wg,
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

	c.log(logger.Info, "opened")

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
	return c.wsconn.RemoteAddr()
}

func (c *webRTCConn) peerConnectionEstablished() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.curPC != nil
}

func (c *webRTCConn) localCandidate() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.curPC != nil {
		var cid string
		for _, stats := range c.curPC.GetStats() {
			if tstats, ok := stats.(webrtc.ICECandidatePairStats); ok && tstats.Nominated {
				cid = tstats.LocalCandidateID
				break
			}
		}

		if cid != "" {
			for _, stats := range c.curPC.GetStats() {
				if tstats, ok := stats.(webrtc.ICECandidateStats); ok && tstats.ID == cid {
					return tstats.CandidateType.String() + "/" + tstats.Protocol + "/" +
						tstats.IP + "/" + strconv.FormatInt(int64(tstats.Port), 10)
				}
			}
		}
	}
	return ""
}

func (c *webRTCConn) remoteCandidate() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.curPC != nil {
		var cid string
		for _, stats := range c.curPC.GetStats() {
			if tstats, ok := stats.(webrtc.ICECandidatePairStats); ok && tstats.Nominated {
				cid = tstats.RemoteCandidateID
				break
			}
		}

		if cid != "" {
			for _, stats := range c.curPC.GetStats() {
				if tstats, ok := stats.(webrtc.ICECandidateStats); ok && tstats.ID == cid {
					return tstats.CandidateType.String() + "/" + tstats.Protocol + "/" +
						tstats.IP + "/" + strconv.FormatInt(int64(tstats.Port), 10)
				}
			}
		}
	}
	return ""
}

func (c *webRTCConn) bytesReceived() uint64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.curPC != nil {
		for _, stats := range c.curPC.GetStats() {
			if tstats, ok := stats.(webrtc.TransportStats); ok {
				if tstats.ID == "iceTransport" {
					return tstats.BytesReceived
				}
			}
		}
	}
	return 0
}

func (c *webRTCConn) bytesSent() uint64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.curPC != nil {
		for _, stats := range c.curPC.GetStats() {
			if tstats, ok := stats.(webrtc.TransportStats); ok {
				if tstats.ID == "iceTransport" {
					return tstats.BytesSent
				}
			}
		}
	}
	return 0
}

func (c *webRTCConn) log(level logger.Level, format string, args ...interface{}) {
	c.parent.log(level, "[conn %v] "+format, append([]interface{}{c.wsconn.RemoteAddr()}, args...)...)
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

	c.log(logger.Info, "closed (%v)", err)
}

func (c *webRTCConn) runInner(ctx context.Context) error {
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

	err = c.writeICEServers(c.genICEServers())
	if err != nil {
		return err
	}

	offer, err := c.readOffer()
	if err != nil {
		return err
	}

	configuration := webrtc.Configuration{ICEServers: c.genICEServers()}
	settingsEngine := webrtc.SettingEngine{}

	if len(c.iceHostNAT1To1IPs) != 0 {
		settingsEngine.SetNAT1To1IPs(c.iceHostNAT1To1IPs, webrtc.ICECandidateTypeHost)
	}

	if c.iceUDPMux != nil {
		settingsEngine.SetICEUDPMux(c.iceUDPMux)
	}

	if c.iceTCPMux != nil {
		settingsEngine.SetICETCPMux(c.iceTCPMux)
		settingsEngine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeTCP4})
	}

	pc, err := newPeerConnection(configuration, webrtc.WithSettingEngine(settingsEngine))
	if err != nil {
		return err
	}

	pcConnected := make(chan struct{})
	pcDisconnected := make(chan struct{})
	pcClosed := make(chan struct{})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		select {
		case <-pcClosed:
			return
		default:
		}

		c.log(logger.Debug, "peer connection state: "+state.String())

		switch state {
		case webrtc.PeerConnectionStateConnected:
			close(pcConnected)

		case webrtc.PeerConnectionStateDisconnected:
			close(pcDisconnected)

		case webrtc.PeerConnectionStateClosed:
			close(pcClosed)
		}
	})

	defer func() {
		pc.Close()
		<-pcClosed
	}()

	for _, track := range tracks {
		rtpSender, err := pc.AddTrack(track.webRTCTrack)
		if err != nil {
			return err
		}

		// read incoming RTCP packets in order to make interceptors work
		go func() {
			buf := make([]byte, 1500)
			for {
				_, _, err := rtpSender.Read(buf)
				if err != nil {
					return
				}
			}
		}()
	}

	localCandidate := make(chan *webrtc.ICECandidate)

	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			select {
			case localCandidate <- i:
			case <-pcConnected:
			case <-ctx.Done():
			}
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

	wsReadError := make(chan error)
	remoteCandidate := make(chan *webrtc.ICECandidateInit)

	go func() {
		for {
			candidate, err := c.readCandidate()
			if err != nil {
				select {
				case wsReadError <- err:
				case <-ctx.Done():
				}
				return
			}

			select {
			case remoteCandidate <- candidate:
			case <-pcConnected:
			case <-ctx.Done():
			}
		}
	}()

	t := time.NewTimer(webrtcHandshakeDeadline)
	defer t.Stop()

outer:
	for {
		select {
		case candidate := <-localCandidate:
			c.log(logger.Debug, "local candidate: %+v", candidate)
			c.writeCandidate(candidate)

		case candidate := <-remoteCandidate:
			c.log(logger.Debug, "remote candidate: %+v", candidate.Candidate)
			err = pc.AddICECandidate(*candidate)
			if err != nil {
				return err
			}

		case err := <-wsReadError:
			return err

		case <-t.C:
			return fmt.Errorf("deadline exceeded")

		case <-pcConnected:
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
	c.curPC = pc
	c.mutex.Unlock()

	c.log(logger.Info, "peer connection established, local candidate: %v, remote candidate: %v",
		c.localCandidate(), c.remoteCandidate())

	ringBuffer, _ := ringbuffer.New(uint64(c.readBufferCount))
	defer ringBuffer.Close()

	writeError := make(chan error)

	for _, track := range tracks {
		ctrack := track
		res.stream.readerAdd(c, track.media, track.format, func(dat formatprocessor.Data) {
			ringBuffer.Push(func() {
				ctrack.cb(dat, ctx, writeError)
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

	case err := <-wsReadError:
		return fmt.Errorf("websocket error: %v", err)

	case err := <-writeError:
		return err

	case <-ctx.Done():
		return fmt.Errorf("terminated")
	}
}

func (c *webRTCConn) allocateTracks(medias media.Medias) ([]*webRTCTrack, error) {
	var ret []*webRTCTrack

	var vp9Format *format.VP9
	vp9Media := medias.FindFormat(&vp9Format)

	if vp9Format != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP9,
				ClockRate: uint32(vp9Format.ClockRate()),
			},
			"vp9",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		encoder := &rtpvp9.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		encoder.Init()

		ret = append(ret, &webRTCTrack{
			media:       vp9Media,
			format:      vp9Format,
			webRTCTrack: webRTCTrak,
			cb: func(dat formatprocessor.Data, ctx context.Context, writeError chan error) {
				tdata := dat.(*formatprocessor.DataVP9)

				if tdata.Frame == nil {
					return
				}

				packets, err := encoder.Encode(tdata.Frame, tdata.PTS)
				if err != nil {
					return
				}

				for _, pkt := range packets {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		})
	}

	var vp8Format *format.VP8

	if vp9Format == nil {
		vp8Media := medias.FindFormat(&vp8Format)

		if vp8Format != nil {
			webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeVP8,
					ClockRate: uint32(vp8Format.ClockRate()),
				},
				"vp8",
				"rtspss",
			)
			if err != nil {
				return nil, err
			}

			encoder := &rtpvp8.Encoder{
				PayloadType:    96,
				PayloadMaxSize: webrtcPayloadMaxSize,
			}
			encoder.Init()

			ret = append(ret, &webRTCTrack{
				media:       vp8Media,
				format:      vp8Format,
				webRTCTrack: webRTCTrak,
				cb: func(dat formatprocessor.Data, ctx context.Context, writeError chan error) {
					tdata := dat.(*formatprocessor.DataVP8)

					if tdata.Frame == nil {
						return
					}

					packets, err := encoder.Encode(tdata.Frame, tdata.PTS)
					if err != nil {
						return
					}

					for _, pkt := range packets {
						webRTCTrak.WriteRTP(pkt)
					}
				},
			})
		}
	}

	if vp9Format == nil && vp8Format == nil {
		var h264Format *format.H264
		h264Media := medias.FindFormat(&h264Format)

		if h264Format != nil {
			webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeH264,
					ClockRate: uint32(h264Format.ClockRate()),
				},
				"h264",
				"rtspss",
			)
			if err != nil {
				return nil, err
			}

			encoder := &rtph264.Encoder{
				PayloadType:    96,
				PayloadMaxSize: webrtcPayloadMaxSize,
			}
			encoder.Init()

			var lastPTS time.Duration
			firstNALUReceived := false

			ret = append(ret, &webRTCTrack{
				media:       h264Media,
				format:      h264Format,
				webRTCTrack: webRTCTrak,
				cb: func(dat formatprocessor.Data, ctx context.Context, writeError chan error) {
					tdata := dat.(*formatprocessor.DataH264)

					if tdata.AU == nil {
						return
					}

					if !firstNALUReceived {
						firstNALUReceived = true
						lastPTS = tdata.PTS
					} else {
						if tdata.PTS < lastPTS {
							select {
							case writeError <- fmt.Errorf("WebRTC doesn't support H264 streams with B-frames"):
							case <-ctx.Done():
							}
							return
						}
						lastPTS = tdata.PTS
					}

					packets, err := encoder.Encode(tdata.AU, tdata.PTS)
					if err != nil {
						return
					}

					for _, pkt := range packets {
						webRTCTrak.WriteRTP(pkt)
					}
				},
			})
		}
	}

	var opusFormat *format.Opus
	opusMedia := medias.FindFormat(&opusFormat)

	if opusFormat != nil {
		webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: uint32(opusFormat.ClockRate()),
			},
			"opus",
			"rtspss",
		)
		if err != nil {
			return nil, err
		}

		ret = append(ret, &webRTCTrack{
			media:       opusMedia,
			format:      opusFormat,
			webRTCTrack: webRTCTrak,
			cb: func(dat formatprocessor.Data, ctx context.Context, writeError chan error) {
				for _, pkt := range dat.GetRTPPackets() {
					webRTCTrak.WriteRTP(pkt)
				}
			},
		})
	}

	var g722Format *format.G722

	if opusFormat == nil {
		g722Media := medias.FindFormat(&g722Format)

		if g722Format != nil {
			webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeG722,
					ClockRate: uint32(g722Format.ClockRate()),
				},
				"g722",
				"rtspss",
			)
			if err != nil {
				return nil, err
			}

			ret = append(ret, &webRTCTrack{
				media:       g722Media,
				format:      g722Format,
				webRTCTrack: webRTCTrak,
				cb: func(dat formatprocessor.Data, ctx context.Context, writeError chan error) {
					for _, pkt := range dat.GetRTPPackets() {
						webRTCTrak.WriteRTP(pkt)
					}
				},
			})
		}
	}

	var g711Format *format.G711

	if opusFormat == nil && g722Format == nil {
		g711Media := medias.FindFormat(&g711Format)

		if g711Format != nil {
			var mtyp string
			if g711Format.MULaw {
				mtyp = webrtc.MimeTypePCMU
			} else {
				mtyp = webrtc.MimeTypePCMA
			}

			webRTCTrak, err := webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{
					MimeType:  mtyp,
					ClockRate: uint32(g711Format.ClockRate()),
				},
				"g711",
				"rtspss",
			)
			if err != nil {
				return nil, err
			}

			ret = append(ret, &webRTCTrack{
				media:       g711Media,
				format:      g711Format,
				webRTCTrack: webRTCTrak,
				cb: func(dat formatprocessor.Data, ctx context.Context, writeError chan error) {
					for _, pkt := range dat.GetRTPPackets() {
						webRTCTrak.WriteRTP(pkt)
					}
				},
			})
		}
	}

	if ret == nil {
		return nil, fmt.Errorf(
			"the stream doesn't contain any supported codec (which are currently VP9, VP8, H264, Opus, G722, G711)")
	}

	return ret, nil
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

func (c *webRTCConn) writeICEServers(iceServers []webrtc.ICEServer) error {
	return c.wsconn.WriteJSON(iceServers)
}

func (c *webRTCConn) readOffer() (*webrtc.SessionDescription, error) {
	var offer webrtc.SessionDescription
	err := c.wsconn.ReadJSON(&offer)
	if err != nil {
		return nil, err
	}

	if offer.Type != webrtc.SDPTypeOffer {
		return nil, fmt.Errorf("received SDP is not an offer")
	}

	return &offer, nil
}

func (c *webRTCConn) writeAnswer(answer *webrtc.SessionDescription) error {
	return c.wsconn.WriteJSON(answer)
}

func (c *webRTCConn) writeCandidate(candidate *webrtc.ICECandidate) error {
	return c.wsconn.WriteJSON(candidate)
}

func (c *webRTCConn) readCandidate() (*webrtc.ICECandidateInit, error) {
	var candidate webrtc.ICECandidateInit
	err := c.wsconn.ReadJSON(&candidate)
	if err != nil {
		return nil, err
	}

	return &candidate, err
}

// apiReaderDescribe implements reader.
func (c *webRTCConn) apiReaderDescribe() interface{} {
	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"webRTCConn", c.uuid.String()}
}
