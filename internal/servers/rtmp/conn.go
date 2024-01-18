package rtmp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	pauseAfterAuthError = 2 * time.Second
)

func pathNameAndQuery(inURL *url.URL) (string, url.Values, string) {
	// remove leading and trailing slashes inserted by OBS and some other clients
	tmp := strings.TrimRight(inURL.String(), "/")
	ur, _ := url.Parse(tmp)
	pathName := strings.TrimLeft(ur.Path, "/")
	return pathName, ur.Query(), ur.RawQuery
}

type connState int

const (
	connStateRead connState = iota + 1
	connStatePublish
)

type conn struct {
	parentCtx           context.Context
	isTLS               bool
	rtspAddress         string
	readTimeout         conf.StringDuration
	writeTimeout        conf.StringDuration
	writeQueueSize      int
	runOnConnect        string
	runOnConnectRestart bool
	runOnDisconnect     string
	wg                  *sync.WaitGroup
	nconn               net.Conn
	externalCmdPool     *externalcmd.Pool
	pathManager         defs.PathManager
	parent              *Server

	ctx       context.Context
	ctxCancel func()
	uuid      uuid.UUID
	created   time.Time
	mutex     sync.RWMutex
	rconn     *rtmp.Conn
	state     connState
	pathName  string
	query     string
}

func (c *conn) initialize() {
	c.ctx, c.ctxCancel = context.WithCancel(c.parentCtx)

	c.uuid = uuid.New()
	c.created = time.Now()

	c.Log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()
}

func (c *conn) Close() {
	c.ctxCancel()
}

func (c *conn) remoteAddr() net.Addr {
	return c.nconn.RemoteAddr()
}

// Log implements logger.Writer.
func (c *conn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.nconn.RemoteAddr()}, args...)...)
}

func (c *conn) ip() net.IP {
	return c.nconn.RemoteAddr().(*net.TCPAddr).IP
}

func (c *conn) run() { //nolint:dupl
	defer c.wg.Done()

	onDisconnectHook := hooks.OnConnect(hooks.OnConnectParams{
		Logger:              c,
		ExternalCmdPool:     c.externalCmdPool,
		RunOnConnect:        c.runOnConnect,
		RunOnConnectRestart: c.runOnConnectRestart,
		RunOnDisconnect:     c.runOnDisconnect,
		RTSPAddress:         c.rtspAddress,
		Desc:                c.APIReaderDescribe(),
	})
	defer onDisconnectHook()

	err := c.runInner()

	c.ctxCancel()

	c.parent.closeConn(c)

	c.Log(logger.Info, "closed: %v", err)
}

func (c *conn) runInner() error {
	readerErr := make(chan error)
	go func() {
		readerErr <- c.runReader()
	}()

	select {
	case err := <-readerErr:
		c.nconn.Close()
		return err

	case <-c.ctx.Done():
		c.nconn.Close()
		<-readerErr
		return errors.New("terminated")
	}
}

func (c *conn) runReader() error {
	c.nconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
	c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
	conn, u, publish, err := rtmp.NewServerConn(c.nconn)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	c.rconn = conn
	c.mutex.Unlock()

	if !publish {
		return c.runRead(conn, u)
	}
	return c.runPublish(conn, u)
}

func (c *conn) runRead(conn *rtmp.Conn, u *url.URL) error {
	pathName, query, rawQuery := pathNameAndQuery(u)

	res := c.pathManager.AddReader(defs.PathAddReaderReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:  pathName,
			Query: rawQuery,
			IP:    c.ip(),
			User:  query.Get("user"),
			Pass:  query.Get("pass"),
			Proto: defs.AuthProtocolRTMP,
			ID:    &c.uuid,
		},
	})

	if res.Err != nil {
		var terr defs.AuthenticationError
		if errors.As(res.Err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(pauseAfterAuthError)
			return terr
		}
		return res.Err
	}

	defer res.Path.RemoveReader(defs.PathRemoveReaderReq{Author: c})

	c.mutex.Lock()
	c.state = connStateRead
	c.pathName = pathName
	c.query = rawQuery
	c.mutex.Unlock()

	writer := asyncwriter.New(c.writeQueueSize, c)

	defer res.Stream.RemoveReader(writer)

	var w *rtmp.Writer

	videoFormat := c.setupVideo(
		&w,
		res.Stream,
		writer)

	audioFormat := c.setupAudio(
		&w,
		res.Stream,
		writer)

	if videoFormat == nil && audioFormat == nil {
		return fmt.Errorf(
			"the stream doesn't contain any supported codec, which are currently H264, MPEG-4 Audio, MPEG-1/2 Audio")
	}

	c.Log(logger.Info, "is reading from path '%s', %s",
		res.Path.Name(), defs.FormatsInfo(res.Stream.FormatsForReader(writer)))

	onUnreadHook := hooks.OnRead(hooks.OnReadParams{
		Logger:          c,
		ExternalCmdPool: c.externalCmdPool,
		Conf:            res.Path.SafeConf(),
		ExternalCmdEnv:  res.Path.ExternalCmdEnv(),
		Reader:          c.APISourceDescribe(),
		Query:           rawQuery,
	})
	defer onUnreadHook()

	var err error
	w, err = rtmp.NewWriter(conn, videoFormat, audioFormat)
	if err != nil {
		return err
	}

	// disable read deadline
	c.nconn.SetReadDeadline(time.Time{})

	writer.Start()

	select {
	case <-c.ctx.Done():
		writer.Stop()
		return fmt.Errorf("terminated")

	case err := <-writer.Error():
		return err
	}
}

func (c *conn) setupVideo(
	w **rtmp.Writer,
	stream *stream.Stream,
	writer *asyncwriter.Writer,
) format.Format {
	var videoFormatH264 *format.H264
	videoMedia := stream.Desc().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		var videoDTSExtractor *h264.DTSExtractor

		stream.AddReader(writer, videoMedia, videoFormatH264, func(u unit.Unit) error {
			tunit := u.(*unit.H264)

			if tunit.AU == nil {
				return nil
			}

			idrPresent := false
			nonIDRPresent := false

			for _, nalu := range tunit.AU {
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeIDR:
					idrPresent = true

				case h264.NALUTypeNonIDR:
					nonIDRPresent = true
				}
			}

			var dts time.Duration

			// wait until we receive an IDR
			if videoDTSExtractor == nil {
				if !idrPresent {
					return nil
				}

				videoDTSExtractor = h264.NewDTSExtractor()

				var err error
				dts, err = videoDTSExtractor.Extract(tunit.AU, tunit.PTS)
				if err != nil {
					return err
				}
			} else {
				if !idrPresent && !nonIDRPresent {
					return nil
				}

				var err error
				dts, err = videoDTSExtractor.Extract(tunit.AU, tunit.PTS)
				if err != nil {
					return err
				}
			}

			c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
			return (*w).WriteH264(tunit.PTS, dts, idrPresent, tunit.AU)
		})

		return videoFormatH264
	}

	return nil
}

func (c *conn) setupAudio(
	w **rtmp.Writer,
	stream *stream.Stream,
	writer *asyncwriter.Writer,
) format.Format {
	var audioFormatMPEG4Audio *format.MPEG4Audio
	audioMedia := stream.Desc().FindFormat(&audioFormatMPEG4Audio)

	if audioMedia != nil {
		stream.AddReader(writer, audioMedia, audioFormatMPEG4Audio, func(u unit.Unit) error {
			tunit := u.(*unit.MPEG4Audio)

			if tunit.AUs == nil {
				return nil
			}

			for i, au := range tunit.AUs {
				c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
				err := (*w).WriteMPEG4Audio(
					tunit.PTS+time.Duration(i)*mpeg4audio.SamplesPerAccessUnit*
						time.Second/time.Duration(audioFormatMPEG4Audio.ClockRate()),
					au,
				)
				if err != nil {
					return err
				}
			}

			return nil
		})

		return audioFormatMPEG4Audio
	}

	var audioFormatMPEG1 *format.MPEG1Audio
	audioMedia = stream.Desc().FindFormat(&audioFormatMPEG1)

	if audioMedia != nil {
		stream.AddReader(writer, audioMedia, audioFormatMPEG1, func(u unit.Unit) error {
			tunit := u.(*unit.MPEG1Audio)

			pts := tunit.PTS

			for _, frame := range tunit.Frames {
				var h mpeg1audio.FrameHeader
				err := h.Unmarshal(frame)
				if err != nil {
					return err
				}

				if !(!h.MPEG2 && h.Layer == 3) {
					return fmt.Errorf("RTMP only supports MPEG-1 layer 3 audio")
				}

				c.nconn.SetWriteDeadline(time.Now().Add(time.Duration(c.writeTimeout)))
				err = (*w).WriteMPEG1Audio(pts, &h, frame)
				if err != nil {
					return err
				}

				pts += time.Duration(h.SampleCount()) *
					time.Second / time.Duration(h.SampleRate)
			}

			return nil
		})

		return audioFormatMPEG1
	}

	return nil
}

func (c *conn) runPublish(conn *rtmp.Conn, u *url.URL) error {
	pathName, query, rawQuery := pathNameAndQuery(u)

	res := c.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:    pathName,
			Query:   rawQuery,
			Publish: true,
			IP:      c.ip(),
			User:    query.Get("user"),
			Pass:    query.Get("pass"),
			Proto:   defs.AuthProtocolRTMP,
			ID:      &c.uuid,
		},
	})

	if res.Err != nil {
		var terr defs.AuthenticationError
		if errors.As(res.Err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(pauseAfterAuthError)
			return terr
		}
		return res.Err
	}

	defer res.Path.RemovePublisher(defs.PathRemovePublisherReq{Author: c})

	c.mutex.Lock()
	c.state = connStatePublish
	c.pathName = pathName
	c.query = rawQuery
	c.mutex.Unlock()

	r, err := rtmp.NewReader(conn)
	if err != nil {
		return err
	}
	videoFormat, audioFormat := r.Tracks()

	var medias []*description.Media
	var stream *stream.Stream

	if videoFormat != nil {
		videoMedia := &description.Media{
			Type:    description.MediaTypeVideo,
			Formats: []format.Format{videoFormat},
		}
		medias = append(medias, videoMedia)

		switch videoFormat.(type) {
		case *format.AV1:
			r.OnDataAV1(func(pts time.Duration, tu [][]byte) {
				stream.WriteUnit(videoMedia, videoFormat, &unit.AV1{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					TU: tu,
				})
			})

		case *format.VP9:
			r.OnDataVP9(func(pts time.Duration, frame []byte) {
				stream.WriteUnit(videoMedia, videoFormat, &unit.VP9{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Frame: frame,
				})
			})

		case *format.H265:
			r.OnDataH265(func(pts time.Duration, au [][]byte) {
				stream.WriteUnit(videoMedia, videoFormat, &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AU: au,
				})
			})

		case *format.H264:
			r.OnDataH264(func(pts time.Duration, au [][]byte) {
				stream.WriteUnit(videoMedia, videoFormat, &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AU: au,
				})
			})

		default:
			return fmt.Errorf("unsupported video codec: %T", videoFormat)
		}
	}

	if audioFormat != nil { //nolint:dupl
		audioMedia := &description.Media{
			Type:    description.MediaTypeAudio,
			Formats: []format.Format{audioFormat},
		}
		medias = append(medias, audioMedia)

		switch audioFormat.(type) {
		case *format.MPEG4Audio:
			r.OnDataMPEG4Audio(func(pts time.Duration, au []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					AUs: [][]byte{au},
				})
			})

		case *format.MPEG1Audio:
			r.OnDataMPEG1Audio(func(pts time.Duration, frame []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Frames: [][]byte{frame},
				})
			})

		case *format.G711:
			r.OnDataG711(func(pts time.Duration, samples []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &unit.G711{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Samples: samples,
				})
			})

		case *format.LPCM:
			r.OnDataLPCM(func(pts time.Duration, samples []byte) {
				stream.WriteUnit(audioMedia, audioFormat, &unit.LPCM{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts,
					},
					Samples: samples,
				})
			})

		default:
			return fmt.Errorf("unsupported audio codec: %T", audioFormat)
		}
	}

	rres := res.Path.StartPublisher(defs.PathStartPublisherReq{
		Author:             c,
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
	})
	if rres.Err != nil {
		return rres.Err
	}

	stream = rres.Stream

	// disable write deadline to allow outgoing acknowledges
	c.nconn.SetWriteDeadline(time.Time{})

	for {
		c.nconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

// APIReaderDescribe implements reader.
func (c *conn) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: func() string {
			if c.isTLS {
				return "rtmpsConn"
			}
			return "rtmpConn"
		}(),
		ID: c.uuid.String(),
	}
}

// APISourceDescribe implements source.
func (c *conn) APISourceDescribe() defs.APIPathSourceOrReader {
	return c.APIReaderDescribe()
}

func (c *conn) apiItem() *defs.APIRTMPConn {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	bytesReceived := uint64(0)
	bytesSent := uint64(0)

	if c.rconn != nil {
		bytesReceived = c.rconn.BytesReceived()
		bytesSent = c.rconn.BytesSent()
	}

	return &defs.APIRTMPConn{
		ID:         c.uuid,
		Created:    c.created,
		RemoteAddr: c.remoteAddr().String(),
		State: func() defs.APIRTMPConnState {
			switch c.state {
			case connStateRead:
				return defs.APIRTMPConnStateRead

			case connStatePublish:
				return defs.APIRTMPConnStatePublish

			default:
				return defs.APIRTMPConnStateIdle
			}
		}(),
		Path:          c.pathName,
		Query:         c.query,
		BytesReceived: bytesReceived,
		BytesSent:     bytesSent,
	}
}
