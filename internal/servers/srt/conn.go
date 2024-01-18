package srt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	mcmpegts "github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	pauseAfterAuthError = 2 * time.Second
)

func srtCheckPassphrase(connReq srt.ConnRequest, passphrase string) error {
	if passphrase == "" {
		return nil
	}

	if !connReq.IsEncrypted() {
		return fmt.Errorf("connection is encrypted, but not passphrase is defined in configuration")
	}

	err := connReq.SetPassphrase(passphrase)
	if err != nil {
		return fmt.Errorf("invalid passphrase")
	}

	return nil
}

type connState int

const (
	connStateRead connState = iota + 1
	connStatePublish
)

type conn struct {
	parentCtx           context.Context
	rtspAddress         string
	readTimeout         conf.StringDuration
	writeTimeout        conf.StringDuration
	writeQueueSize      int
	udpMaxPayloadSize   int
	connReq             srt.ConnRequest
	runOnConnect        string
	runOnConnectRestart bool
	runOnDisconnect     string
	wg                  *sync.WaitGroup
	externalCmdPool     *externalcmd.Pool
	pathManager         defs.PathManager
	parent              *Server

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	mutex     sync.RWMutex
	state     connState
	pathName  string
	query     string
	sconn     srt.Conn

	chNew     chan srtNewConnReq
	chSetConn chan srt.Conn
}

func (c *conn) initialize() {
	c.ctx, c.ctxCancel = context.WithCancel(c.parentCtx)

	c.created = time.Now()
	c.uuid = uuid.New()
	c.chNew = make(chan srtNewConnReq)
	c.chSetConn = make(chan srt.Conn)

	c.Log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()
}

func (c *conn) Close() {
	c.ctxCancel()
}

// Log implements logger.Writer.
func (c *conn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.connReq.RemoteAddr()}, args...)...)
}

func (c *conn) ip() net.IP {
	return c.connReq.RemoteAddr().(*net.UDPAddr).IP
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
	var req srtNewConnReq
	select {
	case req = <-c.chNew:
	case <-c.ctx.Done():
		return errors.New("terminated")
	}

	answerSent, err := c.runInner2(req)

	if !answerSent {
		req.res <- nil
	}

	return err
}

func (c *conn) runInner2(req srtNewConnReq) (bool, error) {
	var streamID streamID
	err := streamID.unmarshal(req.connReq.StreamId())
	if err != nil {
		return false, fmt.Errorf("invalid stream ID '%s': %w", req.connReq.StreamId(), err)
	}

	if streamID.mode == streamIDModePublish {
		return c.runPublish(req, &streamID)
	}
	return c.runRead(req, &streamID)
}

func (c *conn) runPublish(req srtNewConnReq, streamID *streamID) (bool, error) {
	res := c.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:    streamID.path,
			IP:      c.ip(),
			Publish: true,
			User:    streamID.user,
			Pass:    streamID.pass,
			Proto:   defs.AuthProtocolSRT,
			ID:      &c.uuid,
			Query:   streamID.query,
		},
	})

	if res.Err != nil {
		var terr defs.AuthenticationError
		if errors.As(res.Err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(pauseAfterAuthError)
			return false, terr
		}
		return false, res.Err
	}

	defer res.Path.RemovePublisher(defs.PathRemovePublisherReq{Author: c})

	err := srtCheckPassphrase(req.connReq, res.Path.SafeConf().SRTPublishPassphrase)
	if err != nil {
		return false, err
	}

	sconn, err := c.exchangeRequestWithConn(req)
	if err != nil {
		return true, err
	}

	c.mutex.Lock()
	c.state = connStatePublish
	c.pathName = streamID.path
	c.query = streamID.query
	c.sconn = sconn
	c.mutex.Unlock()

	readerErr := make(chan error)
	go func() {
		readerErr <- c.runPublishReader(sconn, res.Path)
	}()

	select {
	case err := <-readerErr:
		sconn.Close()
		return true, err

	case <-c.ctx.Done():
		sconn.Close()
		<-readerErr
		return true, errors.New("terminated")
	}
}

func (c *conn) runPublishReader(sconn srt.Conn, path defs.Path) error {
	sconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
	r, err := mcmpegts.NewReader(mcmpegts.NewBufferedReader(sconn))
	if err != nil {
		return err
	}

	decodeErrLogger := logger.NewLimitedLogger(c)

	r.OnDecodeError(func(err error) {
		decodeErrLogger.Log(logger.Warn, err.Error())
	})

	var stream *stream.Stream

	medias, err := mpegts.ToStream(r, &stream)
	if err != nil {
		return err
	}

	rres := path.StartPublisher(defs.PathStartPublisherReq{
		Author:             c,
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
	})
	if rres.Err != nil {
		return rres.Err
	}

	stream = rres.Stream

	for {
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

func (c *conn) runRead(req srtNewConnReq, streamID *streamID) (bool, error) {
	res := c.pathManager.AddReader(defs.PathAddReaderReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:  streamID.path,
			IP:    c.ip(),
			User:  streamID.user,
			Pass:  streamID.pass,
			Proto: defs.AuthProtocolSRT,
			ID:    &c.uuid,
			Query: streamID.query,
		},
	})

	if res.Err != nil {
		var terr defs.AuthenticationError
		if errors.As(res.Err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(pauseAfterAuthError)
			return false, res.Err
		}
		return false, res.Err
	}

	defer res.Path.RemoveReader(defs.PathRemoveReaderReq{Author: c})

	err := srtCheckPassphrase(req.connReq, res.Path.SafeConf().SRTReadPassphrase)
	if err != nil {
		return false, err
	}

	sconn, err := c.exchangeRequestWithConn(req)
	if err != nil {
		return true, err
	}
	defer sconn.Close()

	c.mutex.Lock()
	c.state = connStateRead
	c.pathName = streamID.path
	c.query = streamID.query
	c.sconn = sconn
	c.mutex.Unlock()

	writer := asyncwriter.New(c.writeQueueSize, c)

	defer res.Stream.RemoveReader(writer)

	bw := bufio.NewWriterSize(sconn, srtMaxPayloadSize(c.udpMaxPayloadSize))

	err = mpegts.FromStream(res.Stream, writer, bw, sconn, time.Duration(c.writeTimeout))
	if err != nil {
		return true, err
	}

	c.Log(logger.Info, "is reading from path '%s', %s",
		res.Path.Name(), defs.FormatsInfo(res.Stream.FormatsForReader(writer)))

	onUnreadHook := hooks.OnRead(hooks.OnReadParams{
		Logger:          c,
		ExternalCmdPool: c.externalCmdPool,
		Conf:            res.Path.SafeConf(),
		ExternalCmdEnv:  res.Path.ExternalCmdEnv(),
		Reader:          c.APIReaderDescribe(),
		Query:           streamID.query,
	})
	defer onUnreadHook()

	// disable read deadline
	sconn.SetReadDeadline(time.Time{})

	writer.Start()

	select {
	case <-c.ctx.Done():
		writer.Stop()
		return true, fmt.Errorf("terminated")

	case err := <-writer.Error():
		return true, err
	}
}

func (c *conn) exchangeRequestWithConn(req srtNewConnReq) (srt.Conn, error) {
	req.res <- c

	select {
	case sconn := <-c.chSetConn:
		return sconn, nil

	case <-c.ctx.Done():
		return nil, errors.New("terminated")
	}
}

// new is called by srtListener through srtServer.
func (c *conn) new(req srtNewConnReq) *conn {
	select {
	case c.chNew <- req:
		return <-req.res

	case <-c.ctx.Done():
		return nil
	}
}

// setConn is called by srtListener .
func (c *conn) setConn(sconn srt.Conn) {
	select {
	case c.chSetConn <- sconn:
	case <-c.ctx.Done():
	}
}

// APIReaderDescribe implements reader.
func (c *conn) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "srtConn",
		ID:   c.uuid.String(),
	}
}

// APISourceDescribe implements source.
func (c *conn) APISourceDescribe() defs.APIPathSourceOrReader {
	return c.APIReaderDescribe()
}

func (c *conn) apiItem() *defs.APISRTConn {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	bytesReceived := uint64(0)
	bytesSent := uint64(0)

	if c.sconn != nil {
		var s srt.Statistics
		c.sconn.Stats(&s)
		bytesReceived = s.Accumulated.ByteRecv
		bytesSent = s.Accumulated.ByteSent
	}

	return &defs.APISRTConn{
		ID:         c.uuid,
		Created:    c.created,
		RemoteAddr: c.connReq.RemoteAddr().String(),
		State: func() defs.APISRTConnState {
			switch c.state {
			case connStateRead:
				return defs.APISRTConnStateRead

			case connStatePublish:
				return defs.APISRTConnStatePublish

			default:
				return defs.APISRTConnStateIdle
			}
		}(),
		Path:          c.pathName,
		Query:         c.query,
		BytesReceived: bytesReceived,
		BytesSent:     bytesSent,
	}
}
