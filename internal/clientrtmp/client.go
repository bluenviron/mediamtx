package clientrtmp

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/codec/h264"

	"github.com/aler9/rtsp-simple-server/internal/client"
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

// Parent is implemented by clientman.ClientMan.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnClientClose(client.Client)
	OnClientAnnounce(client.AnnounceReq)
}

// Client is a RTMP client.
type Client struct {
	readTimeout time.Duration
	stats       *stats.Stats
	wg          *sync.WaitGroup
	conn        rtmputils.ConnPair
	parent      Parent

	path client.Path

	// in
	terminate chan struct{}
}

// New allocates a Client.
func New(
	readTimeout time.Duration,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	conn rtmputils.ConnPair,
	parent Parent) *Client {

	c := &Client{
		readTimeout: readTimeout,
		wg:          wg,
		stats:       stats,
		conn:        conn,
		parent:      parent,
		terminate:   make(chan struct{}),
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
	c.parent.Log(level, "[client %s] "+format, append([]interface{}{c.conn.NConn.RemoteAddr().String()}, args...)...)
}

func (c *Client) ip() net.IP {
	return c.conn.NConn.RemoteAddr().(*net.TCPAddr).IP
}

func (c *Client) run() {
	defer c.wg.Done()
	defer c.log(logger.Info, "disconnected")

	if !c.conn.RConn.Publishing {
		c.conn.NConn.Close()
		c.log(logger.Info, "ERR: client is not publishing")
		return
	}

	var videoTrack *gortsplib.Track
	var audioTrack *gortsplib.Track
	var err error
	var tracks gortsplib.Tracks
	var h264Encoder *rtph264.Encoder
	var aacEncoder *rtpaac.Encoder

	metadataDone := make(chan struct{})
	go func() {
		defer close(metadataDone)
		err = func() error {
			videoTrack, audioTrack, err = rtmputils.Metadata(c.conn, c.readTimeout)
			if err != nil {
				return err
			}

			if videoTrack != nil {
				var err error
				h264Encoder, err = rtph264.NewEncoder(96)
				if err != nil {
					return err
				}
				tracks = append(tracks, videoTrack)
			}

			if audioTrack != nil {
				clockRate, _ := audioTrack.ClockRate()
				var err error
				aacEncoder, err = rtpaac.NewEncoder(96, clockRate)
				if err != nil {
					return err
				}
				tracks = append(tracks, audioTrack)
			}

			for i, t := range tracks {
				t.ID = i
			}
			return nil
		}()
	}()

	select {
	case <-metadataDone:
	case <-c.terminate:
		c.conn.NConn.Close()
		<-metadataDone
	}

	if err != nil {
		c.conn.NConn.Close()
		c.log(logger.Info, "ERR: %s", err)

		c.parent.OnClientClose(c)
		<-c.terminate
		return
	}

	err = func() error {
		// remove trailing slash, that is inserted by OBS
		tmp := strings.TrimSuffix(c.conn.RConn.URL.String(), "/")
		ur, _ := url.Parse(tmp)
		pathName := strings.TrimPrefix(ur.Path, "/")

		resc := make(chan client.AnnounceRes)
		c.parent.OnClientAnnounce(client.AnnounceReq{c, pathName, tracks, ur.Query(), resc}) //nolint:govet
		res := <-resc

		if res.Err != nil {
			switch res.Err.(type) {
			case client.ErrAuthNotCritical:
				return res.Err

			case client.ErrAuthCritical:
				// wait some seconds to stop brute force attacks
				select {
				case <-time.After(pauseAfterAuthError):
				case <-c.terminate:
				}
				return res.Err

			default:
				return res.Err
			}
		}

		c.path = res.Path
		return nil
	}()
	if err != nil {
		c.log(logger.Info, "ERR: %s", err)
		c.conn.NConn.Close()

		c.parent.OnClientClose(c)
		<-c.terminate
		return
	}

	func() {
		resc := make(chan struct{})
		c.path.OnClientRecord(client.RecordReq{c, resc}) //nolint:govet
		<-resc

		c.log(logger.Info, "is publishing to path '%s', %d %s",
			c.path.Name(),
			len(tracks),
			func() string {
				if len(tracks) == 1 {
					return "track"
				}
				return "tracks"
			}())
	}()

	readerDone := make(chan error)
	go func() {
		readerDone <- func() error {
			rtcpSenders := rtmputils.NewRTCPSenderSet(tracks, c.path.OnFrame)
			defer rtcpSenders.Close()

			for {
				c.conn.NConn.SetReadDeadline(time.Now().Add(c.readTimeout))
				pkt, err := c.conn.RConn.ReadPacket()
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

					// encode into RTP/H264 format
					frames, err := h264Encoder.Write(pkt.Time+pkt.CTime, nalus)
					if err != nil {
						return err
					}

					for _, f := range frames {
						rtcpSenders.ProcessFrame(videoTrack.ID, time.Now(), gortsplib.StreamTypeRTP, f)
						c.path.OnFrame(videoTrack.ID, gortsplib.StreamTypeRTP, f)
					}

				case av.AAC:
					if audioTrack == nil {
						return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
					}

					frames, err := aacEncoder.Write(pkt.Time+pkt.CTime, pkt.Data)
					if err != nil {
						return err
					}

					for _, f := range frames {
						rtcpSenders.ProcessFrame(audioTrack.ID, time.Now(), gortsplib.StreamTypeRTP, f)
						c.path.OnFrame(audioTrack.ID, gortsplib.StreamTypeRTP, f)
					}

				default:
					return fmt.Errorf("ERR: unexpected packet: %v", pkt.Type)
				}
			}
		}()
	}()

	select {
	case err := <-readerDone:
		c.conn.NConn.Close()

		if err != io.EOF {
			c.log(logger.Info, "ERR: %s", err)
		}

		if c.path != nil {
			res := make(chan struct{})
			c.path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
			<-res
			c.path = nil
		}

		c.parent.OnClientClose(c)
		<-c.terminate

	case <-c.terminate:
		c.conn.NConn.Close()
		<-readerDone

		if c.path != nil {
			res := make(chan struct{})
			c.path.OnClientRemove(client.RemoveReq{c, res}) //nolint:govet
			<-res
			c.path = nil
		}
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

// OnReaderFrame implements path.Reader.
func (c *Client) OnReaderFrame(trackID int, streamType gortsplib.StreamType, buf []byte) {
}
