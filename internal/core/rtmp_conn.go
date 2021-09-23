package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/ringbuffer"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"
	"github.com/pion/rtp"

	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtcpsenderset"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
)

const (
	rtmpConnPauseAfterAuthError = 2 * time.Second
)

func pathNameAndQuery(inURL *url.URL) (string, url.Values) {
	// remove leading and trailing slashes inserted by OBS and some other clients
	tmp := strings.TrimRight(inURL.String(), "/")
	ur, _ := url.Parse(tmp)
	pathName := strings.TrimLeft(ur.Path, "/")
	return pathName, ur.Query()
}

type rtmpConnTrackIDPayloadPair struct {
	trackID int
	buf     []byte
}

type rtmpConnPathManager interface {
	OnReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes
	OnPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes
}

type rtmpConnParent interface {
	Log(logger.Level, string, ...interface{})
	OnConnClose(*rtmpConn)
}

type rtmpConn struct {
	id                  string
	rtspAddress         string
	readTimeout         time.Duration
	writeTimeout        time.Duration
	readBufferCount     int
	runOnConnect        string
	runOnConnectRestart bool
	wg                  *sync.WaitGroup
	conn                *rtmp.Conn
	pathManager         rtmpConnPathManager
	parent              rtmpConnParent

	ctx        context.Context
	ctxCancel  func()
	path       *path
	ringBuffer *ringbuffer.RingBuffer // read
	state      gortsplib.ServerSessionState
	stateMutex sync.Mutex
}

func newRTMPConn(
	parentCtx context.Context,
	id string,
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	runOnConnect string,
	runOnConnectRestart bool,
	wg *sync.WaitGroup,
	nconn net.Conn,
	pathManager rtmpConnPathManager,
	parent rtmpConnParent) *rtmpConn {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	c := &rtmpConn{
		id:                  id,
		rtspAddress:         rtspAddress,
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		wg:                  wg,
		conn:                rtmp.NewServerConn(nconn),
		pathManager:         pathManager,
		parent:              parent,
		ctx:                 ctx,
		ctxCancel:           ctxCancel,
	}

	c.log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()

	return c
}

// Close closes a Conn.
func (c *rtmpConn) Close() {
	c.ctxCancel()
}

// ID returns the ID of the Conn.
func (c *rtmpConn) ID() string {
	return c.id
}

// RemoteAddr returns the remote address of the Conn.
func (c *rtmpConn) RemoteAddr() net.Addr {
	return c.conn.NetConn().RemoteAddr()
}

func (c *rtmpConn) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr()}, args...)...)
}

func (c *rtmpConn) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

func (c *rtmpConn) safeState() gortsplib.ServerSessionState {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	return c.state
}

func (c *rtmpConn) run() {
	defer c.wg.Done()
	defer c.log(logger.Info, "closed")

	if c.runOnConnect != "" {
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		onConnectCmd := externalcmd.New(c.runOnConnect, c.runOnConnectRestart, externalcmd.Environment{
			Path: "",
			Port: port,
		})
		defer onConnectCmd.Close()
	}

	ctx, cancel := context.WithCancel(c.ctx)
	runErr := make(chan error)
	go func() {
		runErr <- c.runInner(ctx)
	}()

	select {
	case err := <-runErr:
		cancel()

		if err != io.EOF {
			c.log(logger.Info, "ERR: %s", err)
		}

	case <-c.ctx.Done():
		cancel()
		<-runErr
	}

	c.ctxCancel()

	c.parent.OnConnClose(c)
}

func (c *rtmpConn) runInner(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		c.conn.NetConn().Close()
	}()

	c.conn.NetConn().SetReadDeadline(time.Now().Add(c.readTimeout))
	c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
	err := c.conn.ServerHandshake()
	if err != nil {
		return err
	}

	if c.conn.IsPublishing() {
		return c.runPublish(ctx)
	}
	return c.runRead(ctx)
}

func (c *rtmpConn) runRead(ctx context.Context) error {
	pathName, query := pathNameAndQuery(c.conn.URL())

	res := c.pathManager.OnReaderSetupPlay(pathReaderSetupPlayReq{
		Author:   c,
		PathName: pathName,
		IP:       c.ip(),
		ValidateCredentials: func(pathUser string, pathPass string) error {
			return c.validateCredentials(pathUser, pathPass, query)
		},
	})

	if res.Err != nil {
		if terr, ok := res.Err.(pathErrAuthCritical); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(rtmpConnPauseAfterAuthError)
			return errors.New(terr.Message)
		}
		return res.Err
	}

	c.path = res.Path

	defer func() {
		c.path.OnReaderRemove(pathReaderRemoveReq{Author: c})
	}()

	c.stateMutex.Lock()
	c.state = gortsplib.ServerSessionStateRead
	c.stateMutex.Unlock()

	var videoTrack *gortsplib.Track
	videoTrackID := -1
	var h264Decoder *rtph264.Decoder
	var audioTrack *gortsplib.Track
	audioTrackID := -1
	var audioClockRate int
	var aacDecoder *rtpaac.Decoder

	for i, t := range res.Stream.tracks() {
		if t.IsH264() {
			if videoTrack != nil {
				return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
			}

			videoTrack = t
			videoTrackID = i
			h264Decoder = rtph264.NewDecoder()

		} else if t.IsAAC() {
			if audioTrack != nil {
				return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
			}

			audioTrack = t
			audioTrackID = i
			audioClockRate, _ = audioTrack.ClockRate()
			aacDecoder = rtpaac.NewDecoder(audioClockRate)
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return fmt.Errorf("the stream doesn't contain an H264 track or an AAC track")
	}

	c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
	c.conn.WriteMetadata(videoTrack, audioTrack)

	c.ringBuffer = ringbuffer.New(uint64(c.readBufferCount))

	go func() {
		<-ctx.Done()
		c.ringBuffer.Close()
	}()

	c.path.OnReaderPlay(pathReaderPlayReq{
		Author: c,
	})

	// disable read deadline
	c.conn.NetConn().SetReadDeadline(time.Time{})

	var videoBuf [][]byte
	var videoStartPTS time.Duration
	var videoDTSEst *h264.DTSEstimator
	videoFirstIDRFound := false

	for {
		data, ok := c.ringBuffer.Pull()
		if !ok {
			return fmt.Errorf("terminated")
		}
		pair := data.(rtmpConnTrackIDPayloadPair)

		if videoTrack != nil && pair.trackID == videoTrackID {
			var pkt rtp.Packet
			err := pkt.Unmarshal(pair.buf)
			if err != nil {
				c.log(logger.Warn, "unable to decode RTP packet: %v", err)
				continue
			}

			nalus, pts, err := h264Decoder.DecodeRTP(&pkt)
			if err != nil {
				if err != rtph264.ErrMorePacketsNeeded && err != rtph264.ErrNonStartingPacketAndNoPrevious {
					c.log(logger.Warn, "unable to decode video track: %v", err)
				}
				continue
			}

			for _, nalu := range nalus {
				// remove SPS, PPS and AUD, not needed by RTMP
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
					continue
				}

				videoBuf = append(videoBuf, nalu)
			}

			// RTP marker means that all the NALUs with the same PTS have been received.
			// send them together.
			if pkt.Marker {
				idrPresent := func() bool {
					for _, nalu := range nalus {
						typ := h264.NALUType(nalu[0] & 0x1F)
						if typ == h264.NALUTypeIDR {
							return true
						}
					}
					return false
				}()

				// wait until we receive an IDR
				if !videoFirstIDRFound {
					if !idrPresent {
						videoBuf = nil
						continue
					}

					videoFirstIDRFound = true
					videoStartPTS = pts
					videoDTSEst = h264.NewDTSEstimator()
				}

				data, err := h264.EncodeAVCC(videoBuf)
				if err != nil {
					return err
				}

				pts -= videoStartPTS
				dts := videoDTSEst.Feed(pts)

				c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
				err = c.conn.WritePacket(av.Packet{
					Type:  av.H264,
					Data:  data,
					Time:  dts,
					CTime: pts - dts,
				})
				if err != nil {
					return err
				}

				videoBuf = nil
			}

		} else if audioTrack != nil && pair.trackID == audioTrackID {
			var pkt rtp.Packet
			err := pkt.Unmarshal(pair.buf)
			if err != nil {
				c.log(logger.Warn, "unable to decode RTP packet: %v", err)
				continue
			}

			aus, pts, err := aacDecoder.DecodeRTP(&pkt)
			if err != nil {
				if err != rtpaac.ErrMorePacketsNeeded {
					c.log(logger.Warn, "unable to decode audio track: %v", err)
				}
				continue
			}

			if videoTrack != nil && !videoFirstIDRFound {
				continue
			}

			pts -= videoStartPTS
			if pts < 0 {
				continue
			}

			for _, au := range aus {
				c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
				err := c.conn.WritePacket(av.Packet{
					Type: av.AAC,
					Data: au,
					Time: pts,
				})
				if err != nil {
					return err
				}

				pts += 1000 * time.Second / time.Duration(audioClockRate)
			}
		}
	}
}

func (c *rtmpConn) runPublish(ctx context.Context) error {
	c.conn.NetConn().SetReadDeadline(time.Now().Add(c.readTimeout))
	videoTrack, audioTrack, err := c.conn.ReadMetadata()
	if err != nil {
		return err
	}

	var tracks gortsplib.Tracks
	videoTrackID := -1
	audioTrackID := -1

	var h264Encoder *rtph264.Encoder
	if videoTrack != nil {
		h264Encoder = rtph264.NewEncoder(96, nil, nil, nil)
		videoTrackID = len(tracks)
		tracks = append(tracks, videoTrack)
	}

	var aacEncoder *rtpaac.Encoder
	if audioTrack != nil {
		clockRate, _ := audioTrack.ClockRate()
		aacEncoder = rtpaac.NewEncoder(96, clockRate, nil, nil, nil)
		audioTrackID = len(tracks)
		tracks = append(tracks, audioTrack)
	}

	pathName, query := pathNameAndQuery(c.conn.URL())

	res := c.pathManager.OnPublisherAnnounce(pathPublisherAnnounceReq{
		Author:   c,
		PathName: pathName,
		IP:       c.ip(),
		ValidateCredentials: func(pathUser string, pathPass string) error {
			return c.validateCredentials(pathUser, pathPass, query)
		},
	})

	if res.Err != nil {
		if terr, ok := res.Err.(pathErrAuthCritical); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(rtmpConnPauseAfterAuthError)
			return errors.New(terr.Message)
		}
		return res.Err
	}

	c.path = res.Path

	defer func() {
		c.path.OnPublisherRemove(pathPublisherRemoveReq{Author: c})
	}()

	c.stateMutex.Lock()
	c.state = gortsplib.ServerSessionStatePublish
	c.stateMutex.Unlock()

	// disable write deadline
	c.conn.NetConn().SetWriteDeadline(time.Time{})

	rres := c.path.OnPublisherRecord(pathPublisherRecordReq{
		Author: c,
		Tracks: tracks,
	})
	if rres.Err != nil {
		return rres.Err
	}

	rtcpSenders := rtcpsenderset.New(tracks, rres.Stream.onFrame)
	defer rtcpSenders.Close()

	onFrame := func(trackID int, payload []byte) {
		rtcpSenders.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
		rres.Stream.onFrame(trackID, gortsplib.StreamTypeRTP, payload)
	}

	for {
		c.conn.NetConn().SetReadDeadline(time.Now().Add(c.readTimeout))
		pkt, err := c.conn.ReadPacket()
		if err != nil {
			return err
		}

		switch pkt.Type {
		case av.H264:
			if videoTrack == nil {
				return fmt.Errorf("ERR: received an H264 frame, but track is not set up")
			}

			nalus, err := h264.DecodeAVCC(pkt.Data)
			if err != nil {
				return err
			}

			var outNALUs [][]byte

			for _, nalu := range nalus {
				// remove SPS, PPS and AUD, not needed by RTSP
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
					continue
				}

				outNALUs = append(outNALUs, nalu)
			}

			if len(outNALUs) == 0 {
				continue
			}

			frames, err := h264Encoder.Encode(outNALUs, pkt.Time+pkt.CTime)
			if err != nil {
				return fmt.Errorf("ERR while encoding H264: %v", err)
			}

			for _, frame := range frames {
				onFrame(videoTrackID, frame)
			}

		case av.AAC:
			if audioTrack == nil {
				return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
			}

			frames, err := aacEncoder.Encode([][]byte{pkt.Data}, pkt.Time+pkt.CTime)
			if err != nil {
				return fmt.Errorf("ERR while encoding AAC: %v", err)
			}

			for _, frame := range frames {
				onFrame(audioTrackID, frame)
			}

		default:
			return fmt.Errorf("ERR: unexpected packet: %v", pkt.Type)
		}
	}
}

func (c *rtmpConn) validateCredentials(
	pathUser string,
	pathPass string,
	query url.Values,
) error {
	if query.Get("user") != pathUser ||
		query.Get("pass") != pathPass {
		return pathErrAuthCritical{
			Message: "wrong username or password",
		}
	}

	return nil
}

// OnReaderAccepted implements reader.
func (c *rtmpConn) OnReaderAccepted() {
	c.log(logger.Info, "is reading from path '%s'", c.path.Name())
}

// OnReaderFrame implements reader.
func (c *rtmpConn) OnReaderFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		c.ringBuffer.Push(rtmpConnTrackIDPayloadPair{trackID, payload})
	}
}

// OnReaderAPIDescribe implements reader.
func (c *rtmpConn) OnReaderAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"rtmpConn", c.id}
}

// OnSourceAPIDescribe implements source.
func (c *rtmpConn) OnSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"rtmpConn", c.id}
}

// OnPublisherAccepted implements publisher.
func (c *rtmpConn) OnPublisherAccepted(tracksLen int) {
	c.log(logger.Info, "is publishing to path '%s', %d %s",
		c.path.Name(),
		tracksLen,
		func() string {
			if tracksLen == 1 {
				return "track"
			}
			return "tracks"
		}())
}
