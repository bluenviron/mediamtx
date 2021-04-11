package clientrtmp

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/ringbuffer"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/h264"
	"github.com/aler9/rtsp-simple-server/internal/logger"
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

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}

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
	OnClientSetupPlay(client.SetupPlayReq)
	OnClientAnnounce(client.AnnounceReq)
}

// Parent is implemented by clientman.ClientMan.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnClientClose(client.Client)
}

// Client is a RTMP client.
type Client struct {
	rtspPort            int
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

	// read mode
	ringBuffer *ringbuffer.RingBuffer

	// in
	terminate chan struct{}
}

// New allocates a Client.
func New(
	rtspPort int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	runOnConnect string,
	runOnConnectRestart bool,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	nconn net.Conn,
	pathMan PathMan,
	parent Parent) *Client {

	c := &Client{
		rtspPort:            rtspPort,
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
		terminate:           make(chan struct{}),
	}

	atomic.AddInt64(c.stats.CountClients, 1)
	c.log(logger.Info, "connected (RTMP)")

	c.wg.Add(1)
	go c.run()

	return c
}

// Close closes a Client.
func (c *Client) Close() {
	atomic.AddInt64(c.stats.CountClients, -1)
	close(c.terminate)
}

// IsClient implements client.Client.
func (c *Client) IsClient() {}

// IsSource implements path.source.
func (c *Client) IsSource() {}

func (c *Client) log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[client %s] "+format, append([]interface{}{c.conn.NetConn().RemoteAddr().String()}, args...)...)
}

func (c *Client) ip() net.IP {
	return c.conn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

func (c *Client) run() {
	defer c.wg.Done()
	defer c.log(logger.Info, "disconnected")

	if c.runOnConnect != "" {
		onConnectCmd := externalcmd.New(c.runOnConnect, c.runOnConnectRestart, externalcmd.Environment{
			Path: "",
			Port: strconv.FormatInt(int64(c.rtspPort), 10),
		})
		defer onConnectCmd.Close()
	}

	c.conn.NetConn().SetReadDeadline(time.Now().Add(c.readTimeout))
	c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
	err := c.conn.ServerHandshake()
	if err != nil {
		c.log(logger.Info, "ERR: %s", err)
		c.conn.NetConn().Close()

		c.parent.OnClientClose(c)
		<-c.terminate
		return
	}

	if c.conn.IsPublishing() {
		c.runPublish()
	} else {
		c.runRead()
	}
}

func (c *Client) runRead() {
	var path client.Path
	var videoTrack *gortsplib.Track
	var h264Decoder *rtph264.Decoder
	var audioTrack *gortsplib.Track
	var audioClockRate int
	var aacDecoder *rtpaac.Decoder

	err := func() error {
		pathName, query := pathNameAndQuery(c.conn.URL())

		sres := make(chan client.SetupPlayRes)
		c.pathMan.OnClientSetupPlay(client.SetupPlayReq{c, pathName, query, sres}) //nolint:govet
		res := <-sres

		if res.Err != nil {
			if _, ok := res.Err.(client.ErrAuthCritical); ok {
				// wait some seconds to stop brute force attacks
				select {
				case <-time.After(pauseAfterAuthError):
				case <-c.terminate:
				}
			}
			return res.Err
		}

		path = res.Path

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

		return nil
	}()
	if err != nil {
		c.conn.NetConn().Close()
		c.log(logger.Info, "ERR: %v", err)

		if path != nil {
			res := make(chan struct{})
			path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
			<-res
		}

		c.parent.OnClientClose(c)
		<-c.terminate
		return
	}

	c.ringBuffer = ringbuffer.New(uint64(c.readBufferCount))

	pres := make(chan client.PlayRes)
	path.OnClientPlay(client.PlayReq{c, pres}) //nolint:govet
	<-pres

	c.log(logger.Info, "is reading from path '%s'", path.Name())

	// disable read deadline
	c.conn.NetConn().SetReadDeadline(time.Time{})

	writerDone := make(chan error)
	go func() {
		writerDone <- func() error {
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
		}()
	}()

	select {
	case err := <-writerDone:
		c.conn.NetConn().Close()

		if err != io.EOF {
			c.log(logger.Info, "ERR: %s", err)
		}

		res := make(chan struct{})
		path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
		<-res

		c.parent.OnClientClose(c)
		<-c.terminate

	case <-c.terminate:
		res := make(chan struct{})
		path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
		<-res

		c.ringBuffer.Close()
		c.conn.NetConn().Close()
		<-writerDone
	}
}

func (c *Client) runPublish() {
	var videoTrack *gortsplib.Track
	var audioTrack *gortsplib.Track
	var err error
	var tracks gortsplib.Tracks
	var h264Encoder *rtph264.Encoder
	var aacEncoder *rtpaac.Encoder
	var path client.Path

	setupDone := make(chan struct{})
	go func() {
		defer close(setupDone)
		err = func() error {
			c.conn.NetConn().SetReadDeadline(time.Now().Add(c.readTimeout))
			videoTrack, audioTrack, err = c.conn.ReadMetadata()
			if err != nil {
				return err
			}

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

			resc := make(chan client.AnnounceRes)
			c.pathMan.OnClientAnnounce(client.AnnounceReq{c, pathName, tracks, query, resc}) //nolint:govet
			res := <-resc

			if res.Err != nil {
				if _, ok := res.Err.(client.ErrAuthCritical); ok {
					// wait some seconds to stop brute force attacks
					select {
					case <-time.After(pauseAfterAuthError):
					case <-c.terminate:
					}
				}
				return res.Err
			}

			path = res.Path
			return nil
		}()
	}()

	select {
	case <-setupDone:
	case <-c.terminate:
		c.conn.NetConn().Close()
		<-setupDone
	}

	if err != nil {
		c.conn.NetConn().Close()
		c.log(logger.Info, "ERR: %s", err)

		c.parent.OnClientClose(c)
		<-c.terminate
		return
	}

	// disable write deadline
	c.conn.NetConn().SetWriteDeadline(time.Time{})

	readerDone := make(chan error)
	go func() {
		readerDone <- func() error {
			resc := make(chan client.RecordRes)
			path.OnClientRecord(client.RecordReq{Client: c, Res: resc})
			res := <-resc

			if res.Err != nil {
				return res.Err
			}

			c.log(logger.Info, "is publishing to path '%s', %d %s",
				path.Name(),
				len(tracks),
				func() string {
					if len(tracks) == 1 {
						return "track"
					}
					return "tracks"
				}())

			var onPublishCmd *externalcmd.Cmd
			if path.Conf().RunOnPublish != "" {
				onPublishCmd = externalcmd.New(path.Conf().RunOnPublish,
					path.Conf().RunOnPublishRestart, externalcmd.Environment{
						Path: path.Name(),
						Port: strconv.FormatInt(int64(c.rtspPort), 10),
					})
			}

			defer func(path client.Path) {
				if path.Conf().RunOnPublish != "" {
					onPublishCmd.Close()
				}
			}(path)

			rtcpSenders := rtcpsenderset.New(tracks, res.SP.OnFrame)
			defer rtcpSenders.Close()

			onFrame := func(trackID int, payload []byte) {
				rtcpSenders.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
				res.SP.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
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
						// remove SPS, PPS and AUD, not needed by RTSP / RTMP
						typ := h264.NALUType(nalu[0] & 0x1F)
						switch typ {
						case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
							continue
						}

						outNALUs = append(outNALUs, nalu)
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
		}()
	}()

	select {
	case err := <-readerDone:
		c.conn.NetConn().Close()

		if err != io.EOF {
			c.log(logger.Info, "ERR: %s", err)
		}

		res := make(chan struct{})
		path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
		<-res
		path = nil

		c.parent.OnClientClose(c)
		<-c.terminate

	case <-c.terminate:
		c.conn.NetConn().Close()
		<-readerDone

		res := make(chan struct{})
		path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
		<-res
		path = nil
	}
}

// Authenticate performs an authentication.
func (c *Client) Authenticate(authMethods []headers.AuthMethod,
	pathName string, ips []interface{},
	user string, pass string, req interface{}) error {

	// validate ip
	if ips != nil {
		ip := c.ip()

		if !ipEqualOrInRange(ip, ips) {
			c.log(logger.Info, "ERR: ip '%s' not allowed", ip)

			return client.ErrAuthCritical{&base.Response{ //nolint:govet
				StatusCode: base.StatusUnauthorized,
			}}
		}
	}

	// validate user
	if user != "" {
		values := req.(url.Values)

		if values.Get("user") != user ||
			values.Get("pass") != pass {
			return client.ErrAuthCritical{nil} //nolint:govet
		}
	}

	return nil
}

// OnFrame implements path.Reader.
func (c *Client) OnFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		c.ringBuffer.Push(trackIDPayloadPair{trackID, payload})
	}
}
