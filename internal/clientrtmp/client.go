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
	"github.com/notedit/rtmp/codec/h264"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmputils"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	pauseAfterAuthError = 2 * time.Second
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
	// remove trailing slashes inserted by OBS and some other clients
	tmp := strings.TrimSuffix(inURL.String(), "/")
	ur, _ := url.Parse(tmp)
	pathName := strings.TrimPrefix(ur.Path, "/")
	return pathName, ur.Query()
}

// Parent is implemented by clientman.ClientMan.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnClientClose(client.Client)
	OnClientSetupPlay(client.SetupPlayReq)
	OnClientAnnounce(client.AnnounceReq)
}

// Client is a RTMP client.
type Client struct {
	rtspPort            int
	readTimeout         time.Duration
	writeTimeout        time.Duration
	readBufferCount     int
	runOnConnect        string
	runOnConnectRestart bool
	stats               *stats.Stats
	wg                  *sync.WaitGroup
	conn                *rtmputils.Conn
	parent              Parent

	// read mode only
	h264Decoder *rtph264.Decoder
	videoTrack  *gortsplib.Track
	aacDecoder  *rtpaac.Decoder
	audioTrack  *gortsplib.Track
	ringBuffer  *ringbuffer.RingBuffer

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
	conn *rtmputils.Conn,
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
		conn:                conn,
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

	if c.conn.IsPublishing() {
		c.runPublish()
	} else {
		c.runRead()
	}
}

func (c *Client) runRead() {
	var path client.Path
	var tracks gortsplib.Tracks

	err := func() error {
		pathName, query := pathNameAndQuery(c.conn.URL())

		resc := make(chan client.SetupPlayRes)
		c.parent.OnClientSetupPlay(client.SetupPlayReq{c, pathName, query, resc}) //nolint:govet
		res := <-resc

		if res.Err != nil {
			switch res.Err.(type) {
			case client.ErrAuthCritical:
				// wait some seconds to stop brute force attacks
				select {
				case <-time.After(pauseAfterAuthError):
				case <-c.terminate:
				}
			}
			return res.Err
		}

		path = res.Path
		tracks = res.Tracks

		return nil
	}()
	if err != nil {
		c.log(logger.Info, "ERR: %s", err)
		c.conn.NetConn().Close()

		c.parent.OnClientClose(c)
		<-c.terminate
		return
	}

	var videoTrack *gortsplib.Track
	var h264SPS []byte
	var h264PPS []byte
	var audioTrack *gortsplib.Track
	var aacConfig []byte

	err = func() error {
		for i, t := range tracks {
			if t.IsH264() {
				if videoTrack != nil {
					return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
				}
				videoTrack = t

				var err error
				h264SPS, h264PPS, err = t.ExtractDataH264()
				if err != nil {
					return err
				}

			} else if t.IsAAC() {
				if audioTrack != nil {
					return fmt.Errorf("can't read track %d with RTMP: too many tracks", i+1)
				}
				audioTrack = t

				var err error
				aacConfig, err = t.ExtractDataAAC()
				if err != nil {
					return err
				}
			}
		}

		if videoTrack == nil && audioTrack == nil {
			return fmt.Errorf("unable to find a video or audio track")
		}

		c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
		rtmputils.WriteMetadata(c.conn, videoTrack, audioTrack)

		if videoTrack != nil {
			codec := h264.Codec{
				SPS: map[int][]byte{
					0: h264SPS,
				},
				PPS: map[int][]byte{
					0: h264PPS,
				},
			}
			b := make([]byte, 128)
			var n int
			codec.ToConfig(b, &n)
			b = b[:n]

			c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
			c.conn.WritePacket(av.Packet{
				Type: av.H264DecoderConfig,
				Data: b,
			})

			c.h264Decoder = rtph264.NewDecoder()
			c.videoTrack = videoTrack
		}

		if audioTrack != nil {
			c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
			c.conn.WritePacket(av.Packet{
				Type: av.AACDecoderConfig,
				Data: aacConfig,
			})

			c.aacDecoder = rtpaac.NewDecoder(48000)
			c.audioTrack = audioTrack
		}

		c.ringBuffer = ringbuffer.New(uint64(c.readBufferCount))

		resc := make(chan struct{})
		path.OnClientPlay(client.PlayReq{c, resc}) //nolint:govet
		<-resc

		c.log(logger.Info, "is reading from path '%s'", path.Name())

		return nil
	}()
	if err != nil {
		c.conn.NetConn().Close()
		c.log(logger.Info, "ERR: %v", err)

		res := make(chan struct{})
		path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
		<-res
		path = nil

		c.parent.OnClientClose(c)
		<-c.terminate
	}

	writerDone := make(chan error)
	go func() {
		writerDone <- func() error {
			videoInitialized := false
			var videoStartDTS time.Time
			var videoBuf [][]byte
			var videoPTS time.Duration

			for {
				data, ok := c.ringBuffer.Pull()
				if !ok {
					return fmt.Errorf("terminated")
				}

				now := time.Now()

				switch tdata := data.(type) {
				case *rtph264.NALUAndTimestamp:
					if !videoInitialized {
						videoInitialized = true
						videoStartDTS = now
						videoPTS = tdata.Timestamp
					}

					// aggregate NALUs by PTS
					if tdata.Timestamp != videoPTS {
						pkt := av.Packet{
							Type: av.H264,
							Data: h264.FillNALUsAVCC(videoBuf),
							Time: now.Sub(videoStartDTS),
						}

						c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
						err := c.conn.WritePacket(pkt)
						if err != nil {
							return err
						}

						videoBuf = nil
					}

					videoPTS = tdata.Timestamp
					videoBuf = append(videoBuf, tdata.NALU)

				case *rtpaac.AUAndTimestamp:
					pkt := av.Packet{
						Type: av.AAC,
						Data: tdata.AU,
						Time: tdata.Timestamp,
					}

					c.conn.NetConn().SetWriteDeadline(time.Now().Add(c.writeTimeout))
					err := c.conn.WritePacket(pkt)
					if err != nil {
						return err
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
		path = nil

		c.parent.OnClientClose(c)
		<-c.terminate

	case <-c.terminate:
		res := make(chan struct{})
		path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
		<-res

		c.ringBuffer.Close()
		c.conn.NetConn().Close()
		<-writerDone
		path = nil
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
			videoTrack, audioTrack, err = rtmputils.ReadMetadata(c.conn)
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
			c.parent.OnClientAnnounce(client.AnnounceReq{c, pathName, tracks, query, resc}) //nolint:govet
			res := <-resc

			if res.Err != nil {
				switch res.Err.(type) {
				case client.ErrAuthCritical:
					// wait some seconds to stop brute force attacks
					select {
					case <-time.After(pauseAfterAuthError):
					case <-c.terminate:
					}
				}
				return res.Err
			}

			resc2 := make(chan struct{})
			res.Path.OnClientRecord(client.RecordReq{c, resc2}) //nolint:govet
			<-resc2
			path = res.Path

			c.log(logger.Info, "is publishing to path '%s', %d %s",
				path.Name(),
				len(tracks),
				func() string {
					if len(tracks) == 1 {
						return "track"
					}
					return "tracks"
				}())

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

	readerDone := make(chan error)
	go func() {
		readerDone <- func() error {
			rtcpSenders := rtmputils.NewRTCPSenderSet(tracks, path.OnFrame)
			defer rtcpSenders.Close()

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

					// decode from AVCC format
					nalus, typ := h264.SplitNALUs(pkt.Data)
					if typ != h264.NALU_AVCC {
						return fmt.Errorf("invalid NALU format (%d)", typ)
					}

					for _, nalu := range nalus {
						frames, err := h264Encoder.Encode(&rtph264.NALUAndTimestamp{
							Timestamp: pkt.Time + pkt.CTime,
							NALU:      nalu,
						})
						if err != nil {
							return err
						}

						for _, frame := range frames {
							rtcpSenders.ProcessFrame(videoTrack.ID, time.Now(),
								gortsplib.StreamTypeRTP, frame)
							path.OnFrame(videoTrack.ID, gortsplib.StreamTypeRTP, frame)
						}
					}

				case av.AAC:
					if audioTrack == nil {
						return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
					}

					frame, err := aacEncoder.Encode(&rtpaac.AUAndTimestamp{
						Timestamp: pkt.Time,
						AU:        pkt.Data,
					})
					if err != nil {
						return err
					}

					rtcpSenders.ProcessFrame(audioTrack.ID, time.Now(),
						gortsplib.StreamTypeRTP, frame)
					path.OnFrame(audioTrack.ID, gortsplib.StreamTypeRTP, frame)

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

// OnIncomingFrame implements path.Reader.
func (c *Client) OnIncomingFrame(trackID int, streamType gortsplib.StreamType, buf []byte) {
	if streamType == gortsplib.StreamTypeRTP {
		if c.videoTrack != nil {
			if trackID == c.videoTrack.ID {
				nts, err := c.h264Decoder.Decode(buf)
				if err != nil {
					if err != rtph264.ErrMorePacketsNeeded {
						c.log(logger.Debug, "ERR while decoding video track: %v", err)
					}
					return
				}

				for _, nt := range nts {
					c.ringBuffer.Push(nt)
				}
				return
			}
		}

		if c.audioTrack != nil {
			if trackID == c.audioTrack.ID {
				ats, err := c.aacDecoder.Decode(buf)
				if err != nil {
					c.log(logger.Debug, "ERR while decoding audio track: %v", err)
					return
				}

				for _, at := range ats {
					c.ringBuffer.Push(at)
				}
				return
			}
		}
	}
}
