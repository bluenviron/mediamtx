package hls

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/hls"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	recreateInstancePause = 10 * time.Second
)

func emptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type muxerCloseInstanceReq struct {
	instance *muxerInstance
	err      error
}

type muxer struct {
	variant         conf.HLSVariant
	segmentCount    int
	segmentDuration conf.Duration
	partDuration    conf.Duration
	segmentMaxSize  conf.StringSize
	directory       string
	closeAfter      conf.Duration
	wg              *sync.WaitGroup
	pathName        string
	author          *session
	pathManager     serverPathManager
	parent          *Server

	ctx             context.Context
	ctxCancel       func()
	created         time.Time
	path            defs.Path
	lastRequestTime atomic.Int64
	bytesSent       atomic.Uint64

	mutex                            sync.RWMutex
	instance                         *muxerInstance
	cumulatedOutboundFramesDiscarded uint64

	chCloseInstance chan muxerCloseInstanceReq
}

func (m *muxer) initialize() {
	ctx, ctxCancel := context.WithCancel(context.Background())

	m.ctx = ctx
	m.ctxCancel = ctxCancel
	m.created = time.Now()
	m.lastRequestTime.Store(time.Now().UnixNano())
	m.chCloseInstance = make(chan muxerCloseInstanceReq)

	m.Log(logger.Info, "created %s", func() string {
		if m.author == nil {
			return "automatically"
		}
		return "(requested by " + m.author.remoteAddr + ")"
	}())

	// block first request to getInstance() until the first instance is available
	m.mutex.Lock()

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

	if m.instance != nil {
		m.instance.close()
	}

	m.mutex.Lock()
	m.instance = nil
	m.mutex.Unlock()

	m.Log(logger.Info, "destroyed: %v", err)

	m.parent.closeMuxer(m)
}

func (m *muxer) runInner() error {
	query := ""
	if m.author != nil {
		query = m.author.query
	}

	res, err := m.pathManager.AddReader(defs.PathAddReaderReq{
		Author: m,
		AccessRequest: defs.PathAccessRequest{
			Name:     m.pathName,
			Query:    query,
			SkipAuth: true,
		},
	})
	if err != nil {
		m.mutex.Unlock()
		return err
	}

	m.path = res.Path

	defer m.path.RemoveReader(defs.PathRemoveReaderReq{Author: m})

	tmp, err := m.createInstance(res.Stream)
	if err != nil {
		if m.author != nil || errors.Is(err, hls.ErrNoSupportedCodecs) {
			m.mutex.Unlock()
			return err
		}

		m.Log(logger.Error, "muxer instance crashed: %v", err)
	}

	m.instance = tmp
	m.mutex.Unlock()

	var recreateInstanceTimer *time.Timer

	if m.instance != nil {
		recreateInstanceTimer = emptyTimer()
	} else {
		recreateInstanceTimer = time.NewTimer(recreateInstancePause)
	}

	defer func() {
		recreateInstanceTimer.Stop()
	}()

	var activityCheckTimer *time.Timer
	if m.author != nil {
		activityCheckTimer = time.NewTimer(max(time.Duration(m.closeAfter)/3, 1*time.Second))
	} else {
		activityCheckTimer = emptyTimer()
	}

	defer func() {
		activityCheckTimer.Stop()
	}()

	for {
		select {
		case req := <-m.chCloseInstance:
			if m.instance != req.instance {
				continue
			}

			m.mutex.Lock()
			m.cumulatedOutboundFramesDiscarded += m.instance.reader.OutboundFramesDiscarded()
			m.instance = nil
			m.mutex.Unlock()

			if m.author != nil {
				return req.err
			} else {
				m.Log(logger.Error, "muxer instance crashed: %v", req.err)
			}

			recreateInstanceTimer = time.NewTimer(recreateInstancePause)

		case <-recreateInstanceTimer.C:
			tmp, err = m.createInstance(res.Stream)
			if err != nil {
				m.Log(logger.Error, "muxer instance crashed: %v", err)
				recreateInstanceTimer = time.NewTimer(recreateInstancePause)
				continue
			}

			m.mutex.Lock()
			m.instance = tmp
			m.mutex.Unlock()

		case <-activityCheckTimer.C:
			if time.Since(time.Unix(0, m.lastRequestTime.Load())) >= time.Duration(m.closeAfter) {
				return fmt.Errorf("not used anymore")
			}
			activityCheckTimer = time.NewTimer(max(time.Duration(m.closeAfter)/3, 1*time.Second))

		case <-m.ctx.Done():
			return errors.New("terminated")
		}
	}
}

func (m *muxer) createInstance(strm *stream.Stream) (*muxerInstance, error) {
	mi := &muxerInstance{
		lastRequestTime: &m.lastRequestTime,
		variant:         m.variant,
		segmentCount:    m.segmentCount,
		segmentDuration: m.segmentDuration,
		partDuration:    m.partDuration,
		segmentMaxSize:  m.segmentMaxSize,
		directory:       m.directory,
		pathName:        m.pathName,
		bytesSent:       &m.bytesSent,
		wg:              m.wg,
		stream:          strm,
		server:          m.parent,
		parent:          m,
	}
	err := mi.initialize()
	if err != nil {
		return nil, err
	}
	return mi, nil
}

func (m *muxer) closeInstance(mi *muxerInstance, err error) {
	select {
	case m.chCloseInstance <- muxerCloseInstanceReq{instance: mi, err: err}:
	case <-m.ctx.Done():
	}
}

// APIReaderDescribe implements reader.
func (m *muxer) APIReaderDescribe() *defs.APIPathReader {
	return &defs.APIPathReader{
		Type: defs.APIPathReaderTypeHidden,
		ID:   "",
	}
}

func (m *muxer) getInstance() *muxerInstance {
	m.lastRequestTime.Store(time.Now().UnixNano())

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.instance
}

func (m *muxer) apiItem() *defs.APIHLSMuxer {
	m.mutex.RLock()
	instance := m.instance
	cumulatedOutboundFramesDiscarded := m.cumulatedOutboundFramesDiscarded
	m.mutex.RUnlock()

	outboundFramesDiscarded := cumulatedOutboundFramesDiscarded
	if instance != nil {
		outboundFramesDiscarded += instance.reader.OutboundFramesDiscarded()
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
