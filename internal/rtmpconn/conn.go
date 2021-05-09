package rtmpconn

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/ringbuffer"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"

	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/h264"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/readpublisher"
	"github.com/aler9/rtsp-simple-server/internal/rtcpsenderset"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	pauseAfterAuthError = 2 * time.Second

	// an offset is needed to
	// - avoid negative PTS values
	// - avoid PTS < DTS during startup
	ptsOffset = 2 * time.Second
)

func pathNameAndQuery(inURL *url.URL) (string, url.Values) {
	// remove leading and trailing slashes inserted by OBS and some other clients
	tmp := strings.TrimRight(inURL.String(), "/")
	ur, _ := url.Parse(tmp)
	pathName := strings.TrimLeft(ur.Path, "/")
	return pathName, ur.Query()
}

type trackIDPayloadPair struct {
	trackID int
	buf     []byte
}

// PathMan is implemented by pathman.PathMan.
type PathMan interface {
	OnReadPublisherSetupPlay(readpublisher.SetupPlayReq)
	OnReadPublisherAnnounce(readpublisher.AnnounceReq)
}

// Parent is implemented by rtmpserver.Server.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnConnClose(*Conn)
}

// Conn is a server-side RTMP connection.
type Conn struct {
	rtspAddress         string
	readTimeout         time.Duration
	writeTimeout        time.Duration
	readBufferCount     int
	runOnConnect        string
	runOnConnectRestart bool
	wg                  *sync.WaitGroup
	stats               *stats.Stats
	conn                *rtmp.Conn
	pathMan             PathMan
	parent              Parent

	path       readpublisher.Path
	ringBuffer *ringbuffer.RingBuffer // read

	// in
	terminate       chan struct{}
	parentTerminate chan struct{}
}

// New allocates a Conn.
func New(
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	runOnConnect string,
	runOnConnectRestart bool,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	nconn net.Conn,
	pathMan PathMan,
	parent Parent) *Conn {

	c := &Conn{
		rtspAddress:         rtspAddress,
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		wg:                  wg,
		stats:               stats,
		conn:                rtmp.NewServerConn(nconn),
		pathMan:             pathMan,
		parent:              parent,
		terminate:           make(chan struct{}, 1),
		parentTerminate:     make(chan struct{}),
	}

	c.log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()

	return c
}

// ParentClose closes a Conn.
func (c *Conn) ParentClose() {
	c.log(logger.Info, "closed")
	close(c.parentTerminate)
}

// Close closes a Conn.
func (c *Conn) Close() {
	select {
	case c.terminate <- struct{}{}:
	default:
	}
}

// IsReadPublisher implements readpublisher.ReadPublisher.
func (c *Conn) IsReadPublisher() {}

// IsSource implements source.Source.
func (c *Conn) IsSource() {}

func (c *Conn) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[client %v] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr()}, args...)...)
}

func (c *Conn) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

func (c *Conn) run() {
	defer c.wg.Done()

	if c.runOnConnect != "" {
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		onConnectCmd := externalcmd.New(c.runOnConnect, c.runOnConnectRestart, externalcmd.Environment{
			Path: "",
			Port: port,
		})
		defer onConnectCmd.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	case <-c.terminate:
		cancel()
		<-runErr
	}

	if c.path != nil {
		res := make(chan struct{})
		c.path.OnReadPublisherRemove(readpublisher.RemoveReq{c, res}) //nolint:govet
		<-res
	}

	c.parent.OnConnClose(c)
	<-c.parentTerminate
}

func (c *Conn) runInner(ctx context.Context) error {
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

func (c *Conn) runRead(ctx context.Context) error {
	pathName, query := pathNameAndQuery(c.conn.URL())

	sres := make(chan readpublisher.SetupPlayRes)
	c.pathMan.OnReadPublisherSetupPlay(readpublisher.SetupPlayReq{
		Author:   c,
		PathName: pathName,
		IP:       c.ip(),
		ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
			return c.validateCredentials(pathUser, pathPass, query)
		},
		Res: sres})
	res := <-sres

	if res.Err != nil {
		if _, ok := res.Err.(readpublisher.ErrAuthCritical); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(pauseAfterAuthError)
		}
		return res.Err
	}

	c.path = res.Path

	var videoTrack *gortsplib.Track
	var h264Decoder *rtph264.Decoder
	var audioTrack *gortsplib.Track
	var audioClockRate int
	var aacDecoder *rtpaac.Decoder

	for i, t := range res.Tracks {
		if t.IsH264() {
			if videoTrack != nil {
				return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
			}

			videoTrack = t
			h264Decoder = rtph264.NewDecoder()

		} else if t.IsAAC() {
			if audioTrack != nil {
				return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
			}

			audioTrack = t
			audioClockRate, _ = audioTrack.ClockRate()
			aacDecoder = rtpaac.NewDecoder(audioClockRate)
		}
	}

	if videoTrack == nil && audioTrack == nil {
		return fmt.Errorf("unable to find a video or audio track")
	}

	c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
	c.conn.WriteMetadata(videoTrack, audioTrack)

	c.ringBuffer = ringbuffer.New(uint64(c.readBufferCount))

	go func() {
		<-ctx.Done()
		c.ringBuffer.Close()
	}()

	pres := make(chan readpublisher.PlayRes)
	c.path.OnReadPublisherPlay(readpublisher.PlayReq{c, pres}) //nolint:govet
	<-pres

	c.log(logger.Info, "is reading from path '%s'", c.path.Name())

	// disable read deadline
	c.conn.NetConn().SetReadDeadline(time.Time{})

	var videoBuf [][]byte
	videoDTSEst := h264.NewDTSEstimator()

	for {
		data, ok := c.ringBuffer.Pull()
		if !ok {
			return fmt.Errorf("terminated")
		}
		pair := data.(trackIDPayloadPair)

		if videoTrack != nil && pair.trackID == videoTrack.ID {
			nalus, pts, err := h264Decoder.Decode(pair.buf)
			if err != nil {
				if err != rtph264.ErrMorePacketsNeeded {
					c.log(logger.Warn, "unable to decode video track: %v", err)
				}
				continue
			}

			for _, nalu := range nalus {
				// remove SPS, PPS and AUD, not needed by RTSP
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
					continue
				}

				videoBuf = append(videoBuf, nalu)
			}

			// RTP marker means that all the NALUs with the same PTS have been received.
			// send them together.
			marker := (pair.buf[1] >> 7 & 0x1) > 0
			if marker {
				data, err := h264.EncodeAVCC(videoBuf)
				if err != nil {
					return err
				}

				dts := videoDTSEst.Feed(pts + ptsOffset)
				c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
				err = c.conn.WritePacket(av.Packet{
					Type:  av.H264,
					Data:  data,
					Time:  dts,
					CTime: pts + ptsOffset - dts,
				})
				if err != nil {
					return err
				}

				videoBuf = nil
			}

		} else if audioTrack != nil && pair.trackID == audioTrack.ID {
			aus, pts, err := aacDecoder.Decode(pair.buf)
			if err != nil {
				if err != rtpaac.ErrMorePacketsNeeded {
					c.log(logger.Warn, "unable to decode audio track: %v", err)
				}
				continue
			}

			for i, au := range aus {
				auPTS := pts + ptsOffset + time.Duration(i)*1000*time.Second/time.Duration(audioClockRate)

				c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
				err := c.conn.WritePacket(av.Packet{
					Type: av.AAC,
					Data: au,
					Time: auPTS,
				})
				if err != nil {
					return err
				}
			}
		}
	}
}

func (c *Conn) runPublish(ctx context.Context) error {
	c.conn.NetConn().SetReadDeadline(time.Now().Add(c.readTimeout))
	videoTrack, audioTrack, err := c.conn.ReadMetadata()
	if err != nil {
		return err
	}

	var tracks gortsplib.Tracks
	var h264Encoder *rtph264.Encoder
	var aacEncoder *rtpaac.Encoder

	if videoTrack != nil {
		h264Encoder = rtph264.NewEncoder(96, nil, nil, nil)
		tracks = append(tracks, videoTrack)
	}

	if audioTrack != nil {
		clockRate, _ := audioTrack.ClockRate()
		aacEncoder = rtpaac.NewEncoder(96, clockRate, nil, nil, nil)
		tracks = append(tracks, audioTrack)
	}

	for i, t := range tracks {
		t.ID = i
	}

	pathName, query := pathNameAndQuery(c.conn.URL())

	resc := make(chan readpublisher.AnnounceRes)
	c.pathMan.OnReadPublisherAnnounce(readpublisher.AnnounceReq{
		Author:   c,
		PathName: pathName,
		Tracks:   tracks,
		IP:       c.ip(),
		ValidateCredentials: func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error {
			return c.validateCredentials(pathUser, pathPass, query)
		},
		Res: resc,
	})
	res := <-resc

	if res.Err != nil {
		if _, ok := res.Err.(readpublisher.ErrAuthCritical); ok {
			// wait some seconds to stop brute force attacks
			<-time.After(pauseAfterAuthError)
		}
		return res.Err
	}

	c.path = res.Path

	// disable write deadline
	c.conn.NetConn().SetWriteDeadline(time.Time{})

	rresc := make(chan readpublisher.RecordRes)
	c.path.OnReadPublisherRecord(readpublisher.RecordReq{Author: c, Res: rresc})
	rres := <-rresc

	if rres.Err != nil {
		return rres.Err
	}

	c.log(logger.Info, "is publishing to path '%s', %d %s",
		c.path.Name(),
		len(tracks),
		func() string {
			if len(tracks) == 1 {
				return "track"
			}
			return "tracks"
		}())

	var onPublishCmd *externalcmd.Cmd
	if c.path.Conf().RunOnPublish != "" {
		_, port, _ := net.SplitHostPort(c.rtspAddress)
		onPublishCmd = externalcmd.New(c.path.Conf().RunOnPublish,
			c.path.Conf().RunOnPublishRestart, externalcmd.Environment{
				Path: c.path.Name(),
				Port: port,
			})
	}

	defer func(path readpublisher.Path) {
		if path.Conf().RunOnPublish != "" {
			onPublishCmd.Close()
		}
	}(c.path)

	rtcpSenders := rtcpsenderset.New(tracks, rres.SP.OnFrame)
	defer rtcpSenders.Close()

	onFrame := func(trackID int, payload []byte) {
		rtcpSenders.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
		rres.SP.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
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
				onFrame(videoTrack.ID, frame)
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
				onFrame(audioTrack.ID, frame)
			}

		default:
			return fmt.Errorf("ERR: unexpected packet: %v", pkt.Type)
		}
	}
}

func (c *Conn) validateCredentials(
	pathUser string,
	pathPass string,
	query url.Values,
) error {

	if query.Get("user") != pathUser ||
		query.Get("pass") != pathPass {
		return readpublisher.ErrAuthCritical{}
	}

	return nil
}

// OnFrame implements path.Reader.
func (c *Conn) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		c.ringBuffer.Push(trackIDPayloadPair{trackID, payload})
	}
}
