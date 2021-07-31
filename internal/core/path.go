package core

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func newEmptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type pathParent interface {
	Log(logger.Level, string, ...interface{})
	OnPathSourceReady(*path)
	OnPathClose(*path)
}

type pathRTSPSession interface {
	IsRTSPSession()
}

type sourceRedirect struct{}

func (*sourceRedirect) IsSource() {}

type pathReadPublisherState int

const (
	pathReadPublisherStatePrePlay pathReadPublisherState = iota
	pathReadPublisherStatePlay
	pathReadPublisherStatePreRecord
	pathReadPublisherStateRecord
	pathReadPublisherStatePreRemove
)

type pathSourceState int

const (
	pathSourceStateNotReady pathSourceState = iota
	pathSourceStateCreating
	pathSourceStateReady
)

type pathReadersMap struct {
	mutex sync.RWMutex
	ma    map[readPublisher]struct{}
}

func newPathReadersMap() *pathReadersMap {
	return &pathReadersMap{
		ma: make(map[readPublisher]struct{}),
	}
}

func (m *pathReadersMap) add(r readPublisher) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ma[r] = struct{}{}
}

func (m *pathReadersMap) remove(r readPublisher) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.ma, r)
}

func (m *pathReadersMap) forwardFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		c.OnFrame(trackID, streamType, payload)
	}
}

type path struct {
	rtspAddress     string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	readBufferSize  int
	confName        string
	conf            *conf.PathConf
	name            string
	wg              *sync.WaitGroup
	stats           *stats
	parent          pathParent

	ctx                          context.Context
	ctxCancel                    func()
	readPublishers               map[readPublisher]pathReadPublisherState
	describeRequests             []readPublisherDescribeReq
	setupPlayRequests            []readPublisherSetupPlayReq
	source                       source
	sourceStream                 *gortsplib.ServerStream
	nonRTSPReaders               *pathReadersMap
	onDemandCmd                  *externalcmd.Cmd
	onPublishCmd                 *externalcmd.Cmd
	describeTimer                *time.Timer
	sourceCloseTimer             *time.Timer
	sourceCloseTimerStarted      bool
	sourceState                  pathSourceState
	sourceWg                     sync.WaitGroup
	runOnDemandCloseTimer        *time.Timer
	runOnDemandCloseTimerStarted bool
	closeTimer                   *time.Timer
	closeTimerStarted            bool

	// in
	extSourceSetReady    chan sourceExtSetReadyReq
	extSourceSetNotReady chan sourceExtSetNotReadyReq
	describeReq          chan readPublisherDescribeReq
	setupPlayReq         chan readPublisherSetupPlayReq
	announceReq          chan readPublisherAnnounceReq
	playReq              chan readPublisherPlayReq
	recordReq            chan readPublisherRecordReq
	pauseReq             chan readPublisherPauseReq
	removeReq            chan readPublisherRemoveReq
}

func newPath(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	confName string,
	conf *conf.PathConf,
	name string,
	wg *sync.WaitGroup,
	stats *stats,
	parent pathParent) *path {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pa := &path{
		rtspAddress:           rtspAddress,
		readTimeout:           readTimeout,
		writeTimeout:          writeTimeout,
		readBufferCount:       readBufferCount,
		readBufferSize:        readBufferSize,
		confName:              confName,
		conf:                  conf,
		name:                  name,
		wg:                    wg,
		stats:                 stats,
		parent:                parent,
		ctx:                   ctx,
		ctxCancel:             ctxCancel,
		readPublishers:        make(map[readPublisher]pathReadPublisherState),
		nonRTSPReaders:        newPathReadersMap(),
		describeTimer:         newEmptyTimer(),
		sourceCloseTimer:      newEmptyTimer(),
		runOnDemandCloseTimer: newEmptyTimer(),
		closeTimer:            newEmptyTimer(),
		extSourceSetReady:     make(chan sourceExtSetReadyReq),
		extSourceSetNotReady:  make(chan sourceExtSetNotReadyReq),
		describeReq:           make(chan readPublisherDescribeReq),
		setupPlayReq:          make(chan readPublisherSetupPlayReq),
		announceReq:           make(chan readPublisherAnnounceReq),
		playReq:               make(chan readPublisherPlayReq),
		recordReq:             make(chan readPublisherRecordReq),
		pauseReq:              make(chan readPublisherPauseReq),
		removeReq:             make(chan readPublisherRemoveReq),
	}

	pa.wg.Add(1)
	go pa.run()
	return pa
}

func (pa *path) Close() {
	pa.ctxCancel()
}

// Log is the main logging function.
func (pa *path) Log(level logger.Level, format string, args ...interface{}) {
	pa.parent.Log(level, "[path "+pa.name+"] "+format, args...)
}

// ConfName returns the configuration name of this path.
func (pa *path) ConfName() string {
	return pa.confName
}

// Conf returns the configuration of this path.
func (pa *path) Conf() *conf.PathConf {
	return pa.conf
}

// Name returns the name of this path.
func (pa *path) Name() string {
	return pa.name
}

func (pa *path) run() {
	defer pa.wg.Done()

	if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	} else if pa.hasExternalSource() && !pa.conf.SourceOnDemand {
		pa.startExternalSource()
	}

	var onInitCmd *externalcmd.Cmd
	if pa.conf.RunOnInit != "" {
		pa.Log(logger.Info, "on init command started")
		_, port, _ := net.SplitHostPort(pa.rtspAddress)
		onInitCmd = externalcmd.New(pa.conf.RunOnInit, pa.conf.RunOnInitRestart, externalcmd.Environment{
			Path: pa.name,
			Port: port,
		})
	}

outer:
	for {
		select {
		case <-pa.describeTimer.C:
			for _, req := range pa.describeRequests {
				req.Res <- readPublisherDescribeRes{Err: fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			}
			pa.describeRequests = nil

			for _, req := range pa.setupPlayRequests {
				req.Res <- readPublisherSetupPlayRes{Err: fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			}
			pa.setupPlayRequests = nil

			// set state after removeReadPublisher(), so schedule* works once
			pa.sourceState = pathSourceStateNotReady

			pa.scheduleSourceClose()
			pa.scheduleRunOnDemandClose()
			pa.scheduleClose()

		case <-pa.sourceCloseTimer.C:
			pa.sourceCloseTimerStarted = false
			pa.source.(sourceExternal).Close()
			pa.source = nil

			pa.scheduleClose()

		case <-pa.runOnDemandCloseTimer.C:
			pa.runOnDemandCloseTimerStarted = false
			pa.Log(logger.Info, "on demand command stopped")
			pa.onDemandCmd.Close()
			pa.onDemandCmd = nil

			pa.scheduleClose()

		case <-pa.closeTimer.C:
			break outer

		case req := <-pa.extSourceSetReady:
			pa.sourceStream = gortsplib.NewServerStream(req.Tracks)
			pa.onSourceSetReady()
			req.Res <- sourceExtSetReadyRes{}

		case req := <-pa.extSourceSetNotReady:
			pa.onSourceSetNotReady()
			close(req.Res)

		case req := <-pa.describeReq:
			pa.onReadPublisherDescribe(req)

		case req := <-pa.setupPlayReq:
			pa.onReadPublisherSetupPlay(req)

		case req := <-pa.announceReq:
			pa.onReadPublisherAnnounce(req)

		case req := <-pa.playReq:
			pa.onReadPublisherPlay(req)

		case req := <-pa.recordReq:
			pa.onReadPublisherRecord(req)

		case req := <-pa.pauseReq:
			pa.onReadPublisherPause(req)

		case req := <-pa.removeReq:
			if _, ok := pa.readPublishers[req.Author]; !ok {
				close(req.Res)
				continue
			}

			if pa.readPublishers[req.Author] != pathReadPublisherStatePreRemove {
				pa.removeReadPublisher(req.Author)
			}

			delete(pa.readPublishers, req.Author)
			close(req.Res)

		case <-pa.ctx.Done():
			break outer
		}
	}

	pa.ctxCancel()

	pa.describeTimer.Stop()
	pa.sourceCloseTimer.Stop()
	pa.runOnDemandCloseTimer.Stop()
	pa.closeTimer.Stop()

	if onInitCmd != nil {
		pa.Log(logger.Info, "on init command stopped")
		onInitCmd.Close()
	}

	if source, ok := pa.source.(sourceExternal); ok {
		source.Close()
	}
	pa.sourceWg.Wait()

	if pa.onDemandCmd != nil {
		pa.Log(logger.Info, "on demand command stopped")
		pa.onDemandCmd.Close()
	}

	for _, req := range pa.describeRequests {
		req.Res <- readPublisherDescribeRes{Err: fmt.Errorf("terminated")}
	}

	for _, req := range pa.setupPlayRequests {
		req.Res <- readPublisherSetupPlayRes{Err: fmt.Errorf("terminated")}
	}

	for rp, state := range pa.readPublishers {
		if state != pathReadPublisherStatePreRemove {
			switch state {
			case pathReadPublisherStatePlay:
				atomic.AddInt64(pa.stats.CountReaders, -1)

				if _, ok := rp.(pathRTSPSession); !ok {
					pa.nonRTSPReaders.remove(rp)
				}

			case pathReadPublisherStateRecord:
				atomic.AddInt64(pa.stats.CountPublishers, -1)
			}
			rp.Close()
		}
	}

	pa.parent.OnPathClose(pa)
}

func (pa *path) hasExternalSource() bool {
	return strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") ||
		strings.HasPrefix(pa.conf.Source, "rtmp://")
}

func (pa *path) startExternalSource() {
	if strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") {
		pa.source = newRTSPSource(
			pa.ctx,
			pa.conf.Source,
			pa.conf.SourceProtocolParsed,
			pa.conf.SourceAnyPortEnable,
			pa.conf.SourceFingerprint,
			pa.readTimeout,
			pa.writeTimeout,
			pa.readBufferCount,
			pa.readBufferSize,
			&pa.sourceWg,
			pa.stats,
			pa)
	} else if strings.HasPrefix(pa.conf.Source, "rtmp://") {
		pa.source = newRTMPSource(
			pa.ctx,
			pa.conf.Source,
			pa.readTimeout,
			pa.writeTimeout,
			&pa.sourceWg,
			pa.stats,
			pa)
	}
}

func (pa *path) hasReadPublishers() bool {
	for _, state := range pa.readPublishers {
		if state != pathReadPublisherStatePreRemove {
			return true
		}
	}
	return false
}

func (pa *path) hasReadPublishersNotSources() bool {
	for c, state := range pa.readPublishers {
		if state != pathReadPublisherStatePreRemove && c != pa.source {
			return true
		}
	}
	return false
}

func (pa *path) addReadPublisher(c readPublisher, state pathReadPublisherState) {
	pa.readPublishers[c] = state
}

func (pa *path) removeReadPublisher(rp readPublisher) {
	state := pa.readPublishers[rp]
	pa.readPublishers[rp] = pathReadPublisherStatePreRemove

	switch state {
	case pathReadPublisherStatePlay:
		atomic.AddInt64(pa.stats.CountReaders, -1)

		if _, ok := rp.(pathRTSPSession); !ok {
			pa.nonRTSPReaders.remove(rp)
		}

	case pathReadPublisherStateRecord:
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.onSourceSetNotReady()
	}

	if pa.source == rp {
		pa.source = nil
		pa.sourceStream.Close()
		pa.sourceStream = nil

		// close all readPublishers that are reading or waiting to read
		for orp, state := range pa.readPublishers {
			if state != pathReadPublisherStatePreRemove {
				pa.removeReadPublisher(orp)
				orp.Close()
			}
		}
	}

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()
}

func (pa *path) onSourceSetReady() {
	if pa.sourceState == pathSourceStateCreating {
		pa.describeTimer.Stop()
		pa.describeTimer = newEmptyTimer()
	}

	pa.sourceState = pathSourceStateReady

	for _, req := range pa.describeRequests {
		req.Res <- readPublisherDescribeRes{
			Stream: pa.sourceStream,
		}
	}
	pa.describeRequests = nil

	for _, req := range pa.setupPlayRequests {
		pa.onReadPublisherSetupPlayPost(req)
	}
	pa.setupPlayRequests = nil

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()

	pa.parent.OnPathSourceReady(pa)
}

func (pa *path) onSourceSetNotReady() {
	pa.sourceState = pathSourceStateNotReady

	if pa.onPublishCmd != nil {
		pa.onPublishCmd.Close()
		pa.onPublishCmd = nil
	}

	// close all readPublishers that are reading or waiting to read
	for c, state := range pa.readPublishers {
		if c != pa.source && state != pathReadPublisherStatePreRemove {
			pa.removeReadPublisher(c)
			c.Close()
		}
	}
}

func (pa *path) fixedPublisherStart() {
	if pa.hasExternalSource() {
		// start on-demand source
		if pa.source == nil {
			pa.startExternalSource()

			if pa.sourceState != pathSourceStateCreating {
				pa.describeTimer = time.NewTimer(pa.conf.SourceOnDemandStartTimeout)
				pa.sourceState = pathSourceStateCreating
			}

			// reset timer
		} else if pa.sourceCloseTimerStarted {
			pa.sourceCloseTimer.Stop()
			pa.sourceCloseTimer = time.NewTimer(pa.conf.SourceOnDemandCloseAfter)
		}
	}

	if pa.conf.RunOnDemand != "" {
		// start on-demand command
		if pa.onDemandCmd == nil {
			pa.Log(logger.Info, "on demand command started")
			_, port, _ := net.SplitHostPort(pa.rtspAddress)
			pa.onDemandCmd = externalcmd.New(pa.conf.RunOnDemand, pa.conf.RunOnDemandRestart, externalcmd.Environment{
				Path: pa.name,
				Port: port,
			})

			if pa.sourceState != pathSourceStateCreating {
				pa.describeTimer = time.NewTimer(pa.conf.RunOnDemandStartTimeout)
				pa.sourceState = pathSourceStateCreating
			}

			// reset timer
		} else if pa.runOnDemandCloseTimerStarted {
			pa.runOnDemandCloseTimer.Stop()
			pa.runOnDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
		}
	}
}

func (pa *path) onReadPublisherDescribe(req readPublisherDescribeReq) {
	pa.fixedPublisherStart()
	pa.scheduleClose()

	if _, ok := pa.source.(*sourceRedirect); ok {
		req.Res <- readPublisherDescribeRes{
			Redirect: pa.conf.SourceRedirect,
		}
		return
	}

	switch pa.sourceState {
	case pathSourceStateReady:
		req.Res <- readPublisherDescribeRes{
			Stream: pa.sourceStream,
		}
		return

	case pathSourceStateCreating:
		pa.describeRequests = append(pa.describeRequests, req)
		return

	case pathSourceStateNotReady:
		if pa.conf.Fallback != "" {
			fallbackURL := func() string {
				if strings.HasPrefix(pa.conf.Fallback, "/") {
					ur := base.URL{
						Scheme: req.URL.Scheme,
						User:   req.URL.User,
						Host:   req.URL.Host,
						Path:   pa.conf.Fallback,
					}
					return ur.String()
				}
				return pa.conf.Fallback
			}()
			req.Res <- readPublisherDescribeRes{Redirect: fallbackURL}
			return
		}

		req.Res <- readPublisherDescribeRes{Err: readPublisherErrNoOnePublishing{PathName: pa.name}}
		return
	}
}

func (pa *path) onReadPublisherSetupPlay(req readPublisherSetupPlayReq) {
	pa.fixedPublisherStart()
	pa.scheduleClose()

	switch pa.sourceState {
	case pathSourceStateReady:
		pa.onReadPublisherSetupPlayPost(req)
		return

	case pathSourceStateCreating:
		pa.setupPlayRequests = append(pa.setupPlayRequests, req)
		return

	case pathSourceStateNotReady:
		req.Res <- readPublisherSetupPlayRes{Err: readPublisherErrNoOnePublishing{PathName: pa.name}}
		return
	}
}

func (pa *path) onReadPublisherSetupPlayPost(req readPublisherSetupPlayReq) {
	if _, ok := pa.readPublishers[req.Author]; !ok {
		// prevent on-demand source from closing
		if pa.sourceCloseTimerStarted {
			pa.sourceCloseTimer = newEmptyTimer()
			pa.sourceCloseTimerStarted = false
		}

		// prevent on-demand command from closing
		if pa.runOnDemandCloseTimerStarted {
			pa.runOnDemandCloseTimer = newEmptyTimer()
			pa.runOnDemandCloseTimerStarted = false
		}

		pa.addReadPublisher(req.Author, pathReadPublisherStatePrePlay)
	}

	req.Res <- readPublisherSetupPlayRes{
		Path:   pa,
		Stream: pa.sourceStream,
	}
}

func (pa *path) onReadPublisherPlay(req readPublisherPlayReq) {
	atomic.AddInt64(pa.stats.CountReaders, 1)
	pa.readPublishers[req.Author] = pathReadPublisherStatePlay

	if _, ok := req.Author.(pathRTSPSession); !ok {
		pa.nonRTSPReaders.add(req.Author)
	}

	req.Author.OnReaderAccepted()

	close(req.Res)
}

func (pa *path) onReadPublisherAnnounce(req readPublisherAnnounceReq) {
	if _, ok := pa.readPublishers[req.Author]; ok {
		req.Res <- readPublisherAnnounceRes{Err: fmt.Errorf("already publishing or reading")}
		return
	}

	if pa.hasExternalSource() {
		req.Res <- readPublisherAnnounceRes{Err: fmt.Errorf("path '%s' is assigned to an external source", pa.name)}
		return
	}

	if pa.source != nil {
		if pa.conf.DisablePublisherOverride {
			req.Res <- readPublisherAnnounceRes{Err: fmt.Errorf("another client is already publishing on path '%s'", pa.name)}
			return
		}

		pa.Log(logger.Info, "closing existing publisher")
		curPublisher := pa.source.(readPublisher)
		pa.removeReadPublisher(curPublisher)
		curPublisher.Close()

		// prevent path closure
		if pa.closeTimerStarted {
			pa.closeTimer.Stop()
			pa.closeTimer = newEmptyTimer()
			pa.closeTimerStarted = false
		}
	}

	pa.addReadPublisher(req.Author, pathReadPublisherStatePreRecord)

	pa.source = req.Author
	pa.sourceStream = gortsplib.NewServerStream(req.Tracks)
	req.Res <- readPublisherAnnounceRes{Path: pa}
}

func (pa *path) onReadPublisherRecord(req readPublisherRecordReq) {
	if state, ok := pa.readPublishers[req.Author]; !ok || state != pathReadPublisherStatePreRecord {
		req.Res <- readPublisherRecordRes{Err: fmt.Errorf("not recording anymore")}
		return
	}

	atomic.AddInt64(pa.stats.CountPublishers, 1)
	pa.readPublishers[req.Author] = pathReadPublisherStateRecord

	req.Author.OnPublisherAccepted(len(pa.sourceStream.Tracks()))

	pa.onSourceSetReady()

	if pa.conf.RunOnPublish != "" {
		_, port, _ := net.SplitHostPort(pa.rtspAddress)
		pa.onPublishCmd = externalcmd.New(pa.conf.RunOnPublish, pa.conf.RunOnPublishRestart, externalcmd.Environment{
			Path: pa.name,
			Port: port,
		})
	}

	req.Res <- readPublisherRecordRes{}
}

func (pa *path) onReadPublisherPause(req readPublisherPauseReq) {
	state, ok := pa.readPublishers[req.Author]
	if !ok {
		close(req.Res)
		return
	}

	if state == pathReadPublisherStatePlay {
		atomic.AddInt64(pa.stats.CountReaders, -1)
		pa.readPublishers[req.Author] = pathReadPublisherStatePrePlay

		if _, ok := req.Author.(pathRTSPSession); !ok {
			pa.nonRTSPReaders.remove(req.Author)
		}

	} else if state == pathReadPublisherStateRecord {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.readPublishers[req.Author] = pathReadPublisherStatePreRecord
		pa.onSourceSetNotReady()
	}

	close(req.Res)
}

func (pa *path) scheduleSourceClose() {
	if !pa.hasExternalSource() || !pa.conf.SourceOnDemand || pa.source == nil {
		return
	}

	if pa.sourceCloseTimerStarted ||
		pa.sourceState == pathSourceStateCreating ||
		pa.hasReadPublishers() {
		return
	}

	pa.sourceCloseTimer.Stop()
	pa.sourceCloseTimer = time.NewTimer(pa.conf.SourceOnDemandCloseAfter)
	pa.sourceCloseTimerStarted = true
}

func (pa *path) scheduleRunOnDemandClose() {
	if pa.conf.RunOnDemand == "" || pa.onDemandCmd == nil {
		return
	}

	if pa.runOnDemandCloseTimerStarted ||
		pa.sourceState == pathSourceStateCreating ||
		pa.hasReadPublishersNotSources() {
		return
	}

	pa.runOnDemandCloseTimer.Stop()
	pa.runOnDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
	pa.runOnDemandCloseTimerStarted = true
}

func (pa *path) scheduleClose() {
	if pa.conf.Regexp != nil &&
		!pa.hasReadPublishers() &&
		pa.source == nil &&
		pa.sourceState != pathSourceStateCreating &&
		!pa.sourceCloseTimerStarted &&
		!pa.runOnDemandCloseTimerStarted &&
		!pa.closeTimerStarted {

		pa.closeTimer.Stop()
		pa.closeTimer = time.NewTimer(0)
		pa.closeTimerStarted = true
	}
}

// OnSourceExternalSetReady is called by an external source.
func (pa *path) OnSourceExternalSetReady(req sourceExtSetReadyReq) {
	req.Res = make(chan sourceExtSetReadyRes)
	select {
	case pa.extSourceSetReady <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnSourceExternalSetNotReady is called by an external source.
func (pa *path) OnSourceExternalSetNotReady(req sourceExtSetNotReadyReq) {
	req.Res = make(chan struct{})
	select {
	case pa.extSourceSetNotReady <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnPathManDescribe is called by pathManager (forwarded from a readPublisher).
func (pa *path) OnPathManDescribe(req readPublisherDescribeReq) {
	select {
	case pa.describeReq <- req:
	case <-pa.ctx.Done():
		req.Res <- readPublisherDescribeRes{Err: fmt.Errorf("terminated")}
	}
}

// OnPathManSetupPlay is called by pathManager (forwarded from a readPublisher).
func (pa *path) OnPathManSetupPlay(req readPublisherSetupPlayReq) {
	select {
	case pa.setupPlayReq <- req:
	case <-pa.ctx.Done():
		req.Res <- readPublisherSetupPlayRes{Err: fmt.Errorf("terminated")}
	}
}

// OnPathManAnnounce is called by pathManager (forwarded from a readPublisher).
func (pa *path) OnPathManAnnounce(req readPublisherAnnounceReq) {
	select {
	case pa.announceReq <- req:
	case <-pa.ctx.Done():
		req.Res <- readPublisherAnnounceRes{Err: fmt.Errorf("terminated")}
	}
}

// OnReadPublisherPlay is called by a readPublisher.
func (pa *path) OnReadPublisherPlay(req readPublisherPlayReq) {
	req.Res = make(chan struct{})
	select {
	case pa.playReq <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnReadPublisherRecord is called by a readPublisher.
func (pa *path) OnReadPublisherRecord(req readPublisherRecordReq) readPublisherRecordRes {
	req.Res = make(chan readPublisherRecordRes)
	select {
	case pa.recordReq <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return readPublisherRecordRes{Err: fmt.Errorf("terminated")}
	}
}

// OnReadPublisherPause is called by a readPublisher.
func (pa *path) OnReadPublisherPause(req readPublisherPauseReq) {
	req.Res = make(chan struct{})
	select {
	case pa.pauseReq <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnReadPublisherRemove is called by a readPublisher.
func (pa *path) OnReadPublisherRemove(req readPublisherRemoveReq) {
	req.Res = make(chan struct{})
	select {
	case pa.removeReq <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnFrame is called by a source.
func (pa *path) OnSourceFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	// forward to RTSP readers
	pa.sourceStream.WriteFrame(trackID, streamType, payload)

	// forward to non-RTSP readers
	pa.nonRTSPReaders.forwardFrame(trackID, streamType, payload)
}
