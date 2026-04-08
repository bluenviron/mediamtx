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
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/gin-gonic/gin"
)

const (
	closeCheckPeriod = 1 * time.Second
	recreatePause    = 10 * time.Second
)

func emptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type responseWriterWithCounter struct {
	http.ResponseWriter
	bytesSent *atomic.Uint64
}

func (w *responseWriterWithCounter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytesSent.Add(uint64(n))
	return n, err
}

type muxerGetInstanceRes struct {
	instance                         *muxerInstance
	cumulatedOutboundFramesDiscarded uint64
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
	lastRequestTime atomic.Int64
	bytesSent       atomic.Uint64

	instanceMutex                    sync.RWMutex
	instance                         *muxerInstance
	cumulatedOutboundFramesDiscarded uint64
}

func (m *muxer) initialize() {
	ctx, ctxCancel := context.WithCancel(m.parentCtx)

	m.ctx = ctx
	m.ctxCancel = ctxCancel
	m.created = time.Now()
	m.lastRequestTime.Store(time.Now().UnixNano())
	m.bytesSent.Store(0)

	m.Log(logger.Info, "created %s", func() string {
		if m.remoteAddr == "" {
			return "automatically"
		}
		return "(requested by " + m.remoteAddr + ")"
	}())

	// block first request to getInstance() until the first instance is available
	m.instanceMutex.Lock()

	m.wg.Add(1)
	go m.run()
}

func (m *muxer) Close() {
	m.ctxCancel()
}

// Log implements logger.Writer.
func (m *muxer) Log(level logger.Level, format string, args ...any) {
	m.parent.Log(level, "[muxer %s] "+format, append([]any{m.pathName}, args...)...)
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
	res, err := m.pathManager.AddReader(defs.PathAddReaderReq{
		Author: m,
		AccessRequest: defs.PathAccessRequest{
			Name:     m.pathName,
			Query:    m.query,
			SkipAuth: true,
		},
	})
	if err != nil {
		m.instanceMutex.Unlock()
		return err
	}

	m.path = res.Path

	defer m.path.RemoveReader(defs.PathRemoveReaderReq{Author: m})

	tmp, err := m.createInstance(res.Stream)
	if err != nil {
		if m.remoteAddr != "" || errors.Is(err, hls.ErrNoSupportedCodecs) {
			m.instanceMutex.Unlock()
			return err
		}

		m.Log(logger.Error, err.Error())
	}

	m.instance = tmp
	m.instanceMutex.Unlock()

	defer func() {
		if m.instance != nil {
			m.closeInstance()
		}
	}()

	var instanceError chan error
	var recreateTimer *time.Timer

	if m.instance != nil {
		instanceError = m.instance.errorChan()
		recreateTimer = emptyTimer()
	} else {
		instanceError = make(chan error)
		recreateTimer = time.NewTimer(recreatePause)
	}

	var activityCheckTimer *time.Timer
	if m.remoteAddr != "" {
		activityCheckTimer = time.NewTimer(closeCheckPeriod)
	} else {
		activityCheckTimer = emptyTimer()
	}

	for {
		select {
		case err = <-instanceError:
			if m.remoteAddr != "" {
				return err
			}

			m.Log(logger.Error, err.Error())
			m.closeInstance()
			instanceError = make(chan error)
			recreateTimer = time.NewTimer(recreatePause)

		case <-recreateTimer.C:
			tmp, err = m.createInstance(res.Stream)
			if err != nil {
				m.Log(logger.Error, err.Error())
				recreateTimer = time.NewTimer(recreatePause)
			} else {
				m.instanceMutex.Lock()
				m.instance = tmp
				m.instanceMutex.Unlock()

				instanceError = m.instance.errorChan()
			}

		case <-activityCheckTimer.C:
			t := time.Unix(0, m.lastRequestTime.Load())
			if time.Since(t) >= time.Duration(m.closeAfter) {
				return fmt.Errorf("not used anymore")
			}
			activityCheckTimer = time.NewTimer(closeCheckPeriod)

		case <-m.ctx.Done():
			return errors.New("terminated")
		}
	}
}

func (m *muxer) closeInstance() {
	m.instanceMutex.Lock()
	m.cumulatedOutboundFramesDiscarded += m.instance.reader.OutboundFramesDiscarded()
	var tmp *muxerInstance
	tmp, m.instance = m.instance, nil
	m.instanceMutex.Unlock()

	tmp.close()
}

func (m *muxer) createInstance(strm *stream.Stream) (*muxerInstance, error) {
	mi := &muxerInstance{
		variant:         m.variant,
		segmentCount:    m.segmentCount,
		segmentDuration: m.segmentDuration,
		partDuration:    m.partDuration,
		segmentMaxSize:  m.segmentMaxSize,
		directory:       m.directory,
		pathName:        m.pathName,
		stream:          strm,
		parent:          m,
	}
	err := mi.initialize()
	return mi, err
}

func (m *muxer) getInstance() muxerGetInstanceRes {
	m.instanceMutex.RLock()
	defer m.instanceMutex.RUnlock()

	return muxerGetInstanceRes{
		instance:                         m.instance,
		cumulatedOutboundFramesDiscarded: m.cumulatedOutboundFramesDiscarded,
	}
}

// APIReaderDescribe implements reader.
func (m *muxer) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHLSMuxer,
		ID:   "",
	}
}

func (m *muxer) handleRequest(ctx *gin.Context) {
	m.lastRequestTime.Store(time.Now().UnixNano())

	res := m.getInstance()
	if res.instance == nil {
		ctx.Writer.WriteHeader(http.StatusNotFound)
		return
	}

	w := &responseWriterWithCounter{
		ResponseWriter: ctx.Writer,
		bytesSent:      &m.bytesSent,
	}

	res.instance.handleRequest(w, ctx.Request)
}

func (m *muxer) apiItem() *defs.APIHLSMuxer {
	res := m.getInstance()

	outboundFramesDiscarded := res.cumulatedOutboundFramesDiscarded
	if res.instance != nil {
		outboundFramesDiscarded += res.instance.reader.OutboundFramesDiscarded()
	}

	return &defs.APIHLSMuxer{
		Path:                    m.pathName,
		Created:                 m.created,
		LastRequest:             time.Unix(0, m.lastRequestTime.Load()),
		OutboundBytes:           m.bytesSent.Load(),
		OutboundFramesDiscarded: outboundFramesDiscarded,
		BytesSent:               m.bytesSent.Load(),
	}
}
