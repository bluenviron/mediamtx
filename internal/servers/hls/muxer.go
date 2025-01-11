package hls

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
)

const (
	closeCheckPeriod = 1 * time.Second
	recreatePause    = 10 * time.Second
)

func int64Ptr(v int64) *int64 {
	return &v
}

func emptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type responseWriterWithCounter struct {
	http.ResponseWriter
	bytesSent *uint64
}

func (w *responseWriterWithCounter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	atomic.AddUint64(w.bytesSent, uint64(n))
	return n, err
}

type muxerGetInstanceReq struct {
	res chan *muxerInstance
}

type muxer struct {
	parentCtx       context.Context
	remoteAddr      string
	variant         conf.HLSVariant
	segmentCount    int
	segmentDuration conf.Duration
	partDuration    conf.Duration
	segmentMaxSize  conf.StringSize
	directory       string
	closeAfter      conf.Duration
	wg              *sync.WaitGroup
	pathName        string
	pathManager     serverPathManager
	parent          *Server
	query           string

	ctx             context.Context
	ctxCancel       func()
	created         time.Time
	path            defs.Path
	lastRequestTime *int64
	bytesSent       *uint64

	// in
	chGetInstance chan muxerGetInstanceReq
}

func (m *muxer) initialize() {
	ctx, ctxCancel := context.WithCancel(m.parentCtx)

	m.ctx = ctx
	m.ctxCancel = ctxCancel
	m.created = time.Now()
	m.lastRequestTime = int64Ptr(time.Now().UnixNano())
	m.bytesSent = new(uint64)
	m.chGetInstance = make(chan muxerGetInstanceReq)

	m.Log(logger.Info, "created %s", func() string {
		if m.remoteAddr == "" {
			return "automatically"
		}
		return "(requested by " + m.remoteAddr + ")"
	}())

	m.wg.Add(1)
	go m.run()
}

func (m *muxer) Close() {
	m.ctxCancel()
}

// Log implements logger.Writer.
func (m *muxer) Log(level logger.Level, format string, args ...interface{}) {
	m.parent.Log(level, "[muxer %s] "+format, append([]interface{}{m.pathName}, args...)...)
}

// PathName returns the path name.
func (m *muxer) PathName() string {
	return m.pathName
}

func (m *muxer) run() {
	defer m.wg.Done()

	err := m.runInner()

	m.ctxCancel()

	m.parent.closeMuxer(m)

	m.Log(logger.Info, "destroyed: %v", err)
}

func (m *muxer) runInner() error {
	path, stream, err := m.pathManager.AddReader(defs.PathAddReaderReq{
		Author: m,
		AccessRequest: defs.PathAccessRequest{
			Name:     m.pathName,
			Query:    m.query,
			SkipAuth: true,
		},
	})
	if err != nil {
		return err
	}

	m.path = path

	defer m.path.RemoveReader(defs.PathRemoveReaderReq{Author: m})

	var instanceError chan error
	var recreateTimer *time.Timer

	mi := &muxerInstance{
		variant:         m.variant,
		segmentCount:    m.segmentCount,
		segmentDuration: m.segmentDuration,
		partDuration:    m.partDuration,
		segmentMaxSize:  m.segmentMaxSize,
		directory:       m.directory,
		pathName:        m.pathName,
		stream:          stream,
		bytesSent:       m.bytesSent,
		parent:          m,
	}
	err = mi.initialize()
	if err != nil {
		if m.remoteAddr != "" || errors.Is(err, hls.ErrNoSupportedCodecs) {
			return err
		}

		m.Log(logger.Error, err.Error())
		mi = nil
		instanceError = make(chan error)
		recreateTimer = time.NewTimer(recreatePause)
	} else {
		instanceError = mi.errorChan()
		recreateTimer = emptyTimer()
	}

	defer func() {
		if mi != nil {
			mi.close()
		}
	}()

	var activityCheckTimer *time.Timer
	if m.remoteAddr != "" {
		activityCheckTimer = time.NewTimer(closeCheckPeriod)
	} else {
		activityCheckTimer = emptyTimer()
	}

	for {
		select {
		case req := <-m.chGetInstance:
			req.res <- mi

		case err := <-instanceError:
			if m.remoteAddr != "" {
				return err
			}

			m.Log(logger.Error, err.Error())
			mi.close()
			mi = nil
			instanceError = make(chan error)
			recreateTimer = time.NewTimer(recreatePause)

		case <-recreateTimer.C:
			mi = &muxerInstance{
				variant:         m.variant,
				segmentCount:    m.segmentCount,
				segmentDuration: m.segmentDuration,
				partDuration:    m.partDuration,
				segmentMaxSize:  m.segmentMaxSize,
				directory:       m.directory,
				pathName:        m.pathName,
				stream:          stream,
				bytesSent:       m.bytesSent,
				parent:          m,
			}
			err := mi.initialize()
			if err != nil {
				m.Log(logger.Error, err.Error())
				mi = nil
				recreateTimer = time.NewTimer(recreatePause)
			} else {
				instanceError = mi.errorChan()
			}

		case <-activityCheckTimer.C:
			t := time.Unix(0, atomic.LoadInt64(m.lastRequestTime))
			if time.Since(t) >= time.Duration(m.closeAfter) {
				return fmt.Errorf("not used anymore")
			}
			activityCheckTimer = time.NewTimer(closeCheckPeriod)

		case <-m.ctx.Done():
			return errors.New("terminated")
		}
	}
}

func (m *muxer) getInstance() *muxerInstance {
	atomic.StoreInt64(m.lastRequestTime, time.Now().UnixNano())

	req := muxerGetInstanceReq{res: make(chan *muxerInstance)}

	select {
	case m.chGetInstance <- req:
		return <-req.res

	case <-m.ctx.Done():
		return nil
	}
}

// APIReaderDescribe implements reader.
func (m *muxer) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "hlsMuxer",
		ID:   "",
	}
}

func (m *muxer) apiItem() *defs.APIHLSMuxer {
	return &defs.APIHLSMuxer{
		Path:        m.pathName,
		Created:     m.created,
		LastRequest: time.Unix(0, atomic.LoadInt64(m.lastRequestTime)),
		BytesSent:   atomic.LoadUint64(m.bytesSent),
	}
}
