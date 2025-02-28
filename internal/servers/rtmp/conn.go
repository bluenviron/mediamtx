package rtmp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func pathNameAndQuery(inURL *url.URL) (string, url.Values, string, error) {
	tmp := strings.TrimRight(inURL.String(), "/")
	ur, _ := url.Parse(tmp)
	pathName := strings.TrimLeft(ur.Path, "/")

	if pathName == "" {
		return "", nil, "", errors.New("invalid path name")
	}

	filename := "/app/streamId/streamId.json"
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", nil, "", fmt.Errorf("stream key file not found: %v", err)
	}

	var streams []StreamInfo
	if err := json.Unmarshal(data, &streams); err != nil {
		return "", nil, "", errors.New("invalid stream key file")
	}

	var validStreams []StreamInfo
	for _, stream := range streams {
		if stream.StreamKey == pathName && stream.Available {
			validStreams = append(validStreams, stream)
		}
	}

	if len(validStreams) == 0 {
		return "", nil, "", errors.New("no available stream found")
	}

	sort.Slice(validStreams, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, validStreams[i].CreatedAt)
		timeJ, _ := time.Parse(time.RFC3339, validStreams[j].CreatedAt)
		return timeI.Before(timeJ)
	})

	selectedStream := validStreams[0]

	for i := range streams {
		if streams[i].StreamID == selectedStream.StreamID {
			streams[i].Available = false
			break
		}
	}

	updatedData, err := json.MarshalIndent(streams, "", "  ")
	if err != nil {
		return "", nil, "", errors.New("failed to update stream file")
	}

	if err := os.WriteFile(filename, updatedData, 0644); err != nil {
		return "", nil, "", errors.New("failed to save stream file")
	}

	return selectedStream.StreamID, ur.Query(), ur.RawQuery, nil
}

func deleteStreamByID(streamID string) error {
	filename := "/app/streamId/streamId.json"

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("stream key file not found: %v", err)
	}

	var streams []StreamInfo
	if err := json.Unmarshal(data, &streams); err != nil {
		return errors.New("invalid stream key file")
	}

	newStreams := make([]StreamInfo, 0, len(streams))
	found := false
	for _, stream := range streams {
		if stream.StreamID == streamID {
			found = true
			continue
		}
		newStreams = append(newStreams, stream)
	}

	if !found {
		return errors.New("stream ID not found")
	}

	updatedData, err := json.MarshalIndent(newStreams, "", "  ")
	if err != nil {
		return errors.New("failed to update stream file")
	}

	if err := os.WriteFile(filename, updatedData, 0644); err != nil {
		return errors.New("failed to save stream file")
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
	isTLS               bool
	rtspAddress         string
	readTimeout         conf.Duration
	writeTimeout        conf.Duration
	runOnConnect        string
	runOnConnectRestart bool
	runOnDisconnect     string
	wg                  *sync.WaitGroup
	nconn               net.Conn
	externalCmdPool     *externalcmd.Pool
	pathManager         serverPathManager
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
	if err := deleteStreamByID(c.pathName); err != nil {
		c.Log(logger.Info, "failed to delete stream: %v", err)
	}
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
	pathName, query, rawQuery, err := pathNameAndQuery(u)
	if err != nil {
		return err
	}

	path, stream, err := c.pathManager.AddReader(defs.PathAddReaderReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:  pathName,
			Query: rawQuery,
			IP:    c.ip(),
			User:  query.Get("user"),
			Pass:  query.Get("pass"),
			Proto: auth.ProtocolRTMP,
			ID:    &c.uuid,
		},
	})
	if err != nil {
		var terr *auth.Error
		if errors.As(err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(auth.PauseAfterError)
			return terr
		}
		return err
	}

	defer path.RemoveReader(defs.PathRemoveReaderReq{Author: c})

	c.mutex.Lock()
	c.state = connStateRead
	c.pathName = pathName
	c.query = rawQuery
	c.mutex.Unlock()

	err = rtmp.FromStream(stream, c, conn, c.nconn, time.Duration(c.writeTimeout))
	if err != nil {
		return err
	}

	c.Log(logger.Info, "is reading from path '%s', %s",
		path.Name(), defs.FormatsInfo(stream.ReaderFormats(c)))

	onUnreadHook := hooks.OnRead(hooks.OnReadParams{
		Logger:          c,
		ExternalCmdPool: c.externalCmdPool,
		Conf:            path.SafeConf(),
		ExternalCmdEnv:  path.ExternalCmdEnv(),
		Reader:          c.APISourceDescribe(),
		Query:           rawQuery,
	})
	defer onUnreadHook()

	// disable read deadline
	c.nconn.SetReadDeadline(time.Time{})

	stream.StartReader(c)
	defer stream.RemoveReader(c)

	select {
	case <-c.ctx.Done():
		return fmt.Errorf("terminated")

	case err := <-stream.ReaderError(c):
		return err
	}
}

func (c *conn) runPublish(conn *rtmp.Conn, u *url.URL) error {
	pathName, query, rawQuery, err := pathNameAndQuery(u)
	if err != nil {
		return err
	}

	path, err := c.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:    pathName,
			Query:   rawQuery,
			Publish: true,
			IP:      c.ip(),
			User:    query.Get("user"),
			Pass:    query.Get("pass"),
			Proto:   auth.ProtocolRTMP,
			ID:      &c.uuid,
		},
	})
	if err != nil {
		var terr *auth.Error
		if errors.As(err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(auth.PauseAfterError)
			return terr
		}
		return err
	}

	defer path.RemovePublisher(defs.PathRemovePublisherReq{Author: c})

	c.mutex.Lock()
	c.state = connStatePublish
	c.pathName = pathName
	c.query = rawQuery
	c.mutex.Unlock()

	r, err := rtmp.NewReader(conn)
	if err != nil {
		return err
	}

	var stream *stream.Stream

	medias, err := rtmp.ToStream(r, &stream)
	if err != nil {
		return err
	}

	stream, err = path.StartPublisher(defs.PathStartPublisherReq{
		Author:             c,
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
	})
	if err != nil {
		return err
	}

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
