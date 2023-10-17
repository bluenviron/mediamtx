package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/asyncwriter"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
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

type srtConnState int

const (
	srtConnStateRead srtConnState = iota + 1
	srtConnStatePublish
)

type srtConnPathManager interface {
	addReader(req pathAddReaderReq) pathAddReaderRes
	addPublisher(req pathAddPublisherReq) pathAddPublisherRes
}

type srtConnParent interface {
	logger.Writer
	closeConn(*srtConn)
}

type srtConn struct {
	*conn

	rtspAddress       string
	readTimeout       conf.StringDuration
	writeTimeout      conf.StringDuration
	writeQueueSize    int
	udpMaxPayloadSize int
	connReq           srt.ConnRequest
	wg                *sync.WaitGroup
	externalCmdPool   *externalcmd.Pool
	pathManager       srtConnPathManager
	parent            srtConnParent

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	mutex     sync.RWMutex
	state     srtConnState
	pathName  string
	sconn     srt.Conn

	chNew     chan srtNewConnReq
	chSetConn chan srt.Conn
}

func newSRTConn(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	writeQueueSize int,
	udpMaxPayloadSize int,
	connReq srt.ConnRequest,
	runOnConnect string,
	runOnConnectRestart bool,
	runOnDisconnect string,
	wg *sync.WaitGroup,
	externalCmdPool *externalcmd.Pool,
	pathManager srtConnPathManager,
	parent srtConnParent,
) *srtConn {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	c := &srtConn{
		rtspAddress:       rtspAddress,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		writeQueueSize:    writeQueueSize,
		udpMaxPayloadSize: udpMaxPayloadSize,
		connReq:           connReq,
		wg:                wg,
		externalCmdPool:   externalCmdPool,
		pathManager:       pathManager,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		created:           time.Now(),
		uuid:              uuid.New(),
		chNew:             make(chan srtNewConnReq),
		chSetConn:         make(chan srt.Conn),
	}

	c.conn = newConn(
		rtspAddress,
		runOnConnect,
		runOnConnectRestart,
		runOnDisconnect,
		externalCmdPool,
		c,
	)

	c.Log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()

	return c
}

func (c *srtConn) close() {
	c.ctxCancel()
}

func (c *srtConn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.connReq.RemoteAddr()}, args...)...)
}

func (c *srtConn) ip() net.IP {
	return c.connReq.RemoteAddr().(*net.UDPAddr).IP
}

func (c *srtConn) run() { //nolint:dupl
	defer c.wg.Done()

	desc := c.apiReaderDescribe()
	c.conn.open(desc)
	defer c.conn.close(desc)

	err := c.runInner()

	c.ctxCancel()

	c.parent.closeConn(c)

	c.Log(logger.Info, "closed: %v", err)
}

func (c *srtConn) runInner() error {
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

func (c *srtConn) runInner2(req srtNewConnReq) (bool, error) {
	parts := strings.Split(req.connReq.StreamId(), ":")
	if (len(parts) != 2 && len(parts) != 4) || (parts[0] != "read" && parts[0] != "publish") {
		return false, fmt.Errorf("invalid streamid '%s':"+
			" it must be 'action:pathname' or 'action:pathname:user:pass', "+
			"where action is either read or publish, pathname is the path name, user and pass are the credentials",
			req.connReq.StreamId())
	}

	pathName := parts[1]
	user := ""
	pass := ""

	if len(parts) == 4 {
		user, pass = parts[2], parts[3]
	}

	if parts[0] == "publish" {
		return c.runPublish(req, pathName, user, pass)
	}
	return c.runRead(req, pathName, user, pass)
}

func (c *srtConn) runPublish(req srtNewConnReq, pathName string, user string, pass string) (bool, error) {
	res := c.pathManager.addPublisher(pathAddPublisherReq{
		author: c,
		accessRequest: pathAccessRequest{
			name:    pathName,
			ip:      c.ip(),
			publish: true,
			user:    user,
			pass:    pass,
			proto:   authProtocolSRT,
			id:      &c.uuid,
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			// TODO: re-enable. Currently this freezes the listener.
			// wait some seconds to stop brute force attacks
			// <-time.After(srtPauseAfterAuthError)
			return false, terr
		}
		return false, res.err
	}

	defer res.path.removePublisher(pathRemovePublisherReq{author: c})

	err := srtCheckPassphrase(req.connReq, res.path.conf.SRTPublishPassphrase)
	if err != nil {
		return false, err
	}

	sconn, err := c.exchangeRequestWithConn(req)
	if err != nil {
		return true, err
	}

	c.mutex.Lock()
	c.state = srtConnStatePublish
	c.pathName = pathName
	c.sconn = sconn
	c.mutex.Unlock()

	readerErr := make(chan error)
	go func() {
		readerErr <- c.runPublishReader(sconn, res.path)
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

func (c *srtConn) runPublishReader(sconn srt.Conn, path *path) error {
	sconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
	r, err := mpegts.NewReader(mpegts.NewBufferedReader(sconn))
	if err != nil {
		return err
	}

	decodeErrLogger := logger.NewLimitedLogger(c)

	r.OnDecodeError(func(err error) {
		decodeErrLogger.Log(logger.Warn, err.Error())
	})

	var stream *stream.Stream

	medias, err := mpegtsSetupRead(r, &stream)
	if err != nil {
		return err
	}

	rres := path.startPublisher(pathStartPublisherReq{
		author:             c,
		desc:               &description.Session{Medias: medias},
		generateRTPPackets: true,
	})
	if rres.err != nil {
		return rres.err
	}

	stream = rres.stream

	for {
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

func (c *srtConn) runRead(req srtNewConnReq, pathName string, user string, pass string) (bool, error) {
	res := c.pathManager.addReader(pathAddReaderReq{
		author: c,
		accessRequest: pathAccessRequest{
			name:  pathName,
			ip:    c.ip(),
			user:  user,
			pass:  pass,
			proto: authProtocolSRT,
			id:    &c.uuid,
		},
	})

	if res.err != nil {
		if terr, ok := res.err.(*errAuthentication); ok {
			// TODO: re-enable. Currently this freezes the listener.
			// wait some seconds to stop brute force attacks
			// <-time.After(srtPauseAfterAuthError)
			return false, terr
		}
		return false, res.err
	}

	defer res.path.removeReader(pathRemoveReaderReq{author: c})

	err := srtCheckPassphrase(req.connReq, res.path.conf.SRTReadPassphrase)
	if err != nil {
		return false, err
	}

	sconn, err := c.exchangeRequestWithConn(req)
	if err != nil {
		return true, err
	}
	defer sconn.Close()

	c.mutex.Lock()
	c.state = srtConnStateRead
	c.pathName = pathName
	c.sconn = sconn
	c.mutex.Unlock()

	writer := asyncwriter.New(c.writeQueueSize, c)

	defer res.stream.RemoveReader(writer)

	bw := bufio.NewWriterSize(sconn, srtMaxPayloadSize(c.udpMaxPayloadSize))

	err = mpegtsSetupWrite(res.stream, writer, bw, sconn, time.Duration(c.writeTimeout))
	if err != nil {
		return true, err
	}

	c.Log(logger.Info, "is reading from path '%s', %s",
		res.path.name, readerMediaInfo(writer, res.stream))

	pathConf := res.path.safeConf()

	onUnreadHook := readerOnReadHook(
		c.externalCmdPool,
		pathConf,
		res.path,
		c.apiReaderDescribe(),
		"",
		c,
	)
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

func (c *srtConn) exchangeRequestWithConn(req srtNewConnReq) (srt.Conn, error) {
	req.res <- c

	select {
	case sconn := <-c.chSetConn:
		return sconn, nil

	case <-c.ctx.Done():
		return nil, errors.New("terminated")
	}
}

// new is called by srtListener through srtServer.
func (c *srtConn) new(req srtNewConnReq) *srtConn {
	select {
	case c.chNew <- req:
		return <-req.res

	case <-c.ctx.Done():
		return nil
	}
}

// setConn is called by srtListener .
func (c *srtConn) setConn(sconn srt.Conn) {
	select {
	case c.chSetConn <- sconn:
	case <-c.ctx.Done():
	}
}

// apiReaderDescribe implements reader.
func (c *srtConn) apiReaderDescribe() apiPathSourceOrReader {
	return apiPathSourceOrReader{
		Type: "srtConn",
		ID:   c.uuid.String(),
	}
}

// apiSourceDescribe implements source.
func (c *srtConn) apiSourceDescribe() apiPathSourceOrReader {
	return c.apiReaderDescribe()
}

func (c *srtConn) apiItem() *apiSRTConn {
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

	return &apiSRTConn{
		ID:         c.uuid,
		Created:    c.created,
		RemoteAddr: c.connReq.RemoteAddr().String(),
		State: func() apiSRTConnState {
			switch c.state {
			case srtConnStateRead:
				return apiSRTConnStateRead

			case srtConnStatePublish:
				return apiSRTConnStatePublish

			default:
				return apiSRTConnStateIdle
			}
		}(),
		Path:          c.pathName,
		BytesReceived: bytesReceived,
		BytesSent:     bytesSent,
	}
}
