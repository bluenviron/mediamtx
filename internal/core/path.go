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

	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func newEmptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type pathErrNoOnePublishing struct {
	PathName string
}

// Error implements the error interface.
func (e pathErrNoOnePublishing) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.PathName)
}

type pathErrAuthNotCritical struct {
	*base.Response
}

// Error implements the error interface.
func (pathErrAuthNotCritical) Error() string {
	return "non-critical authentication error"
}

type pathErrAuthCritical struct {
	Message  string
	Response *base.Response
}

// Error implements the error interface.
func (pathErrAuthCritical) Error() string {
	return "critical authentication error"
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

type pathReaderState int

const (
	pathReaderStatePrePlay pathReaderState = iota
	pathReaderStatePlay
)

type pathSourceState int

const (
	pathSourceStateNotReady pathSourceState = iota
	pathSourceStateCreating
	pathSourceStateReady
)

type pathSourceStaticSetReadyReq struct {
	Tracks gortsplib.Tracks
	Res    chan struct{}
}

type pathSourceStaticSetNotReadyReq struct {
	Res chan struct{}
}

type pathReaderRemoveReq struct {
	Author reader
	Res    chan struct{}
}

type pathPublisherRemoveReq struct {
	Author publisher
	Res    chan struct{}
}

type pathDescribeRes struct {
	Stream   *gortsplib.ServerStream
	Redirect string
	Err      error
}

type pathDescribeReq struct {
	PathName            string
	URL                 *base.URL
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan pathDescribeRes
}

type pathReaderSetupPlayRes struct {
	Path   *path
	Stream *gortsplib.ServerStream
	Err    error
}

type pathReaderSetupPlayReq struct {
	Author              reader
	PathName            string
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan pathReaderSetupPlayRes
}

type pathPublisherAnnounceRes struct {
	Path *path
	Err  error
}

type pathPublisherAnnounceReq struct {
	Author              publisher
	PathName            string
	Tracks              gortsplib.Tracks
	IP                  net.IP
	ValidateCredentials func(authMethods []headers.AuthMethod, pathUser string, pathPass string) error
	Res                 chan pathPublisherAnnounceRes
}

type pathReaderPlayReq struct {
	Author reader
	Res    chan struct{}
}

type pathPublisherRecordRes struct {
	Err error
}

type pathPublisherRecordReq struct {
	Author publisher
	Res    chan pathPublisherRecordRes
}

type pathReaderPauseReq struct {
	Author reader
	Res    chan struct{}
}

type pathPublisherPauseReq struct {
	Author publisher
	Res    chan struct{}
}

type pathReadersMap struct {
	mutex sync.RWMutex
	ma    map[reader]struct{}
}

func newPathReadersMap() *pathReadersMap {
	return &pathReadersMap{
		ma: make(map[reader]struct{}),
	}
}

func (m *pathReadersMap) add(r reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.ma[r] = struct{}{}
}

func (m *pathReadersMap) remove(r reader) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.ma, r)
}

func (m *pathReadersMap) forwardFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for c := range m.ma {
		c.OnReaderFrame(trackID, streamType, payload)
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
	readers                      map[reader]pathReaderState
	describeRequests             []pathDescribeReq
	setupPlayRequests            []pathReaderSetupPlayReq
	source                       source
	sourceStaticWg               sync.WaitGroup
	stream                       *gortsplib.ServerStream
	nonRTSPReaders               *pathReadersMap
	onDemandCmd                  *externalcmd.Cmd
	onPublishCmd                 *externalcmd.Cmd
	describeTimer                *time.Timer
	sourceCloseTimer             *time.Timer
	sourceCloseTimerStarted      bool
	sourceState                  pathSourceState
	runOnDemandCloseTimer        *time.Timer
	runOnDemandCloseTimerStarted bool
	closeTimer                   *time.Timer
	closeTimerStarted            bool

	// in
	sourceStaticSetReady    chan pathSourceStaticSetReadyReq
	sourceStaticSetNotReady chan pathSourceStaticSetNotReadyReq
	describe                chan pathDescribeReq
	publisherRemove         chan pathPublisherRemoveReq
	publisherAnnounce       chan pathPublisherAnnounceReq
	publisherRecord         chan pathPublisherRecordReq
	publisherPause          chan pathPublisherPauseReq
	readerRemove            chan pathReaderRemoveReq
	readerSetupPlay         chan pathReaderSetupPlayReq
	readerPlay              chan pathReaderPlayReq
	readerPause             chan pathReaderPauseReq
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
		rtspAddress:             rtspAddress,
		readTimeout:             readTimeout,
		writeTimeout:            writeTimeout,
		readBufferCount:         readBufferCount,
		readBufferSize:          readBufferSize,
		confName:                confName,
		conf:                    conf,
		name:                    name,
		wg:                      wg,
		stats:                   stats,
		parent:                  parent,
		ctx:                     ctx,
		ctxCancel:               ctxCancel,
		readers:                 make(map[reader]pathReaderState),
		nonRTSPReaders:          newPathReadersMap(),
		describeTimer:           newEmptyTimer(),
		sourceCloseTimer:        newEmptyTimer(),
		runOnDemandCloseTimer:   newEmptyTimer(),
		closeTimer:              newEmptyTimer(),
		sourceStaticSetReady:    make(chan pathSourceStaticSetReadyReq),
		sourceStaticSetNotReady: make(chan pathSourceStaticSetNotReadyReq),
		describe:                make(chan pathDescribeReq),
		publisherRemove:         make(chan pathPublisherRemoveReq),
		publisherAnnounce:       make(chan pathPublisherAnnounceReq),
		publisherRecord:         make(chan pathPublisherRecordReq),
		publisherPause:          make(chan pathPublisherPauseReq),
		readerRemove:            make(chan pathReaderRemoveReq),
		readerSetupPlay:         make(chan pathReaderSetupPlayReq),
		readerPlay:              make(chan pathReaderPlayReq),
		readerPause:             make(chan pathReaderPauseReq),
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
	} else if pa.hasStaticSource() && !pa.conf.SourceOnDemand {
		pa.startStaticSource()
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
				req.Res <- pathDescribeRes{Err: fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			}
			pa.describeRequests = nil

			for _, req := range pa.setupPlayRequests {
				req.Res <- pathReaderSetupPlayRes{Err: fmt.Errorf("publisher of path '%s' has timed out", pa.name)}
			}
			pa.setupPlayRequests = nil

			// set state after removeReader(), so schedule* works once
			pa.sourceState = pathSourceStateNotReady

			pa.scheduleSourceClose()
			pa.scheduleRunOnDemandClose()
			pa.scheduleClose()

		case <-pa.sourceCloseTimer.C:
			pa.sourceCloseTimerStarted = false
			pa.source.(sourceStatic).Close()
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

		case req := <-pa.sourceStaticSetReady:
			pa.stream = gortsplib.NewServerStream(req.Tracks)
			pa.onSourceSetReady()
			close(req.Res)

		case req := <-pa.sourceStaticSetNotReady:
			pa.onSourceSetNotReady()
			close(req.Res)

		case req := <-pa.describe:
			pa.onDescribe(req)

		case req := <-pa.publisherRemove:
			pa.onPublisherRemove(req)

		case req := <-pa.publisherAnnounce:
			pa.onPublisherAnnounce(req)

		case req := <-pa.publisherRecord:
			pa.onPublisherRecord(req)

		case req := <-pa.publisherPause:
			pa.onPublisherPause(req)

		case req := <-pa.readerRemove:
			pa.onReaderRemove(req)

		case req := <-pa.readerSetupPlay:
			pa.onReaderSetupPlay(req)

		case req := <-pa.readerPlay:
			pa.onReaderPlay(req)

		case req := <-pa.readerPause:
			pa.onReaderPause(req)

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

	for _, req := range pa.describeRequests {
		req.Res <- pathDescribeRes{Err: fmt.Errorf("terminated")}
	}

	for _, req := range pa.setupPlayRequests {
		req.Res <- pathReaderSetupPlayRes{Err: fmt.Errorf("terminated")}
	}

	for rp, state := range pa.readers {
		if state == pathReaderStatePlay {
			atomic.AddInt64(pa.stats.CountReaders, -1)

			if _, ok := rp.(pathRTSPSession); !ok {
				pa.nonRTSPReaders.remove(rp)
			}
		}
		rp.Close()
	}

	if pa.onDemandCmd != nil {
		pa.Log(logger.Info, "on demand command stopped")
		pa.onDemandCmd.Close()
	}

	if pa.source != nil {
		if source, ok := pa.source.(sourceStatic); ok {
			source.Close()
			pa.sourceStaticWg.Wait()
		} else if source, ok := pa.source.(publisher); ok {
			if pa.sourceState == pathSourceStateReady {
				atomic.AddInt64(pa.stats.CountPublishers, -1)
			}
			source.Close()
		}
	}

	pa.parent.OnPathClose(pa)
}

func (pa *path) hasStaticSource() bool {
	return strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") ||
		strings.HasPrefix(pa.conf.Source, "rtmp://")
}

func (pa *path) startStaticSource() {
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
			&pa.sourceStaticWg,
			pa.stats,
			pa)
	} else if strings.HasPrefix(pa.conf.Source, "rtmp://") {
		pa.source = newRTMPSource(
			pa.ctx,
			pa.conf.Source,
			pa.readTimeout,
			pa.writeTimeout,
			&pa.sourceStaticWg,
			pa.stats,
			pa)
	}
}

func (pa *path) removeReader(r reader) {
	state := pa.readers[r]

	if state == pathReaderStatePlay {
		atomic.AddInt64(pa.stats.CountReaders, -1)

		if _, ok := r.(pathRTSPSession); !ok {
			pa.nonRTSPReaders.remove(r)
		}
	}

	delete(pa.readers, r)

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()
}

func (pa *path) removePublisher(p publisher) {
	if pa.sourceState == pathSourceStateReady {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.onSourceSetNotReady()
	}

	pa.source = nil
	pa.stream.Close()
	pa.stream = nil

	for r := range pa.readers {
		pa.removeReader(r)
		r.Close()
	}

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()
}

func (pa *path) fixedPublisherStart() {
	if pa.hasStaticSource() {
		// start on-demand source
		if pa.source == nil {
			pa.startStaticSource()

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

func (pa *path) onSourceSetReady() {
	if pa.sourceState == pathSourceStateCreating {
		pa.describeTimer.Stop()
		pa.describeTimer = newEmptyTimer()
	}

	pa.sourceState = pathSourceStateReady

	for _, req := range pa.describeRequests {
		req.Res <- pathDescribeRes{
			Stream: pa.stream,
		}
	}
	pa.describeRequests = nil

	for _, req := range pa.setupPlayRequests {
		pa.onReaderSetupPlayPost(req)
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

	for r := range pa.readers {
		pa.removeReader(r)
		r.Close()
	}
}

func (pa *path) onDescribe(req pathDescribeReq) {
	pa.fixedPublisherStart()
	pa.scheduleClose()

	if _, ok := pa.source.(*sourceRedirect); ok {
		req.Res <- pathDescribeRes{
			Redirect: pa.conf.SourceRedirect,
		}
		return
	}

	switch pa.sourceState {
	case pathSourceStateReady:
		req.Res <- pathDescribeRes{
			Stream: pa.stream,
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
			req.Res <- pathDescribeRes{Redirect: fallbackURL}
			return
		}

		req.Res <- pathDescribeRes{Err: pathErrNoOnePublishing{PathName: pa.name}}
		return
	}
}

func (pa *path) onPublisherRemove(req pathPublisherRemoveReq) {
	if pa.source == req.Author {
		pa.removePublisher(req.Author)
	}
	close(req.Res)
}

func (pa *path) onPublisherAnnounce(req pathPublisherAnnounceReq) {
	if pa.source != nil {
		if pa.hasStaticSource() {
			req.Res <- pathPublisherAnnounceRes{Err: fmt.Errorf("path '%s' is assigned to a static source", pa.name)}
			return
		}

		if pa.conf.DisablePublisherOverride {
			req.Res <- pathPublisherAnnounceRes{Err: fmt.Errorf("another publisher is already publishing to path '%s'", pa.name)}
			return
		}

		pa.Log(logger.Info, "closing existing publisher")
		curPub := pa.source.(publisher)
		pa.removePublisher(curPub)
		curPub.Close()

		// prevent path closure
		if pa.closeTimerStarted {
			pa.closeTimer.Stop()
			pa.closeTimer = newEmptyTimer()
			pa.closeTimerStarted = false
		}
	}

	pa.source = req.Author
	pa.stream = gortsplib.NewServerStream(req.Tracks)
	req.Res <- pathPublisherAnnounceRes{Path: pa}
}

func (pa *path) onPublisherRecord(req pathPublisherRecordReq) {
	if pa.source != req.Author {
		req.Res <- pathPublisherRecordRes{Err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	atomic.AddInt64(pa.stats.CountPublishers, 1)

	req.Author.OnPublisherAccepted(len(pa.stream.Tracks()))

	pa.onSourceSetReady()

	if pa.conf.RunOnPublish != "" {
		_, port, _ := net.SplitHostPort(pa.rtspAddress)
		pa.onPublishCmd = externalcmd.New(pa.conf.RunOnPublish, pa.conf.RunOnPublishRestart, externalcmd.Environment{
			Path: pa.name,
			Port: port,
		})
	}

	req.Res <- pathPublisherRecordRes{}
}

func (pa *path) onPublisherPause(req pathPublisherPauseReq) {
	if req.Author == pa.source && pa.sourceState == pathSourceStateReady {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.onSourceSetNotReady()
	}
	close(req.Res)
}

func (pa *path) onReaderRemove(req pathReaderRemoveReq) {
	if _, ok := pa.readers[req.Author]; ok {
		pa.removeReader(req.Author)
	}
	close(req.Res)
}

func (pa *path) onReaderSetupPlay(req pathReaderSetupPlayReq) {
	pa.fixedPublisherStart()
	pa.scheduleClose()

	switch pa.sourceState {
	case pathSourceStateReady:
		pa.onReaderSetupPlayPost(req)
		return

	case pathSourceStateCreating:
		pa.setupPlayRequests = append(pa.setupPlayRequests, req)
		return

	case pathSourceStateNotReady:
		req.Res <- pathReaderSetupPlayRes{Err: pathErrNoOnePublishing{PathName: pa.name}}
		return
	}
}

func (pa *path) onReaderSetupPlayPost(req pathReaderSetupPlayReq) {
	if _, ok := pa.readers[req.Author]; !ok {
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

		pa.readers[req.Author] = pathReaderStatePrePlay
	}

	req.Res <- pathReaderSetupPlayRes{
		Path:   pa,
		Stream: pa.stream,
	}
}

func (pa *path) onReaderPlay(req pathReaderPlayReq) {
	atomic.AddInt64(pa.stats.CountReaders, 1)
	pa.readers[req.Author] = pathReaderStatePlay

	if _, ok := req.Author.(pathRTSPSession); !ok {
		pa.nonRTSPReaders.add(req.Author)
	}

	req.Author.OnReaderAccepted()

	close(req.Res)
}

func (pa *path) onReaderPause(req pathReaderPauseReq) {
	if state, ok := pa.readers[req.Author]; ok && state == pathReaderStatePlay {
		atomic.AddInt64(pa.stats.CountReaders, -1)
		pa.readers[req.Author] = pathReaderStatePrePlay

		if _, ok := req.Author.(pathRTSPSession); !ok {
			pa.nonRTSPReaders.remove(req.Author)
		}
	}
	close(req.Res)
}

func (pa *path) scheduleSourceClose() {
	if !pa.hasStaticSource() || !pa.conf.SourceOnDemand || pa.source == nil {
		return
	}

	if pa.sourceCloseTimerStarted ||
		pa.sourceState == pathSourceStateCreating ||
		len(pa.readers) > 0 ||
		pa.source != nil {
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
		len(pa.readers) > 0 {
		return
	}

	pa.runOnDemandCloseTimer.Stop()
	pa.runOnDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
	pa.runOnDemandCloseTimerStarted = true
}

func (pa *path) scheduleClose() {
	if pa.conf.Regexp != nil &&
		len(pa.readers) == 0 &&
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

// OnSourceStaticSetReady is called by a sourceStatic.
func (pa *path) OnSourceStaticSetReady(req pathSourceStaticSetReadyReq) {
	req.Res = make(chan struct{})
	select {
	case pa.sourceStaticSetReady <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnSourceStaticSetNotReady is called by a sourceStatic.
func (pa *path) OnSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq) {
	req.Res = make(chan struct{})
	select {
	case pa.sourceStaticSetNotReady <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnDescribe is called by pathManager (asynchronous).
func (pa *path) OnDescribe(req pathDescribeReq) {
	select {
	case pa.describe <- req:
	case <-pa.ctx.Done():
		req.Res <- pathDescribeRes{Err: fmt.Errorf("terminated")}
	}
}

// OnPublisherRemove is called by a publisher.
func (pa *path) OnPublisherRemove(req pathPublisherRemoveReq) {
	req.Res = make(chan struct{})
	select {
	case pa.publisherRemove <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnPublisherAnnounce is called by pathManager (asynchronous).
func (pa *path) OnPublisherAnnounce(req pathPublisherAnnounceReq) {
	select {
	case pa.publisherAnnounce <- req:
	case <-pa.ctx.Done():
		req.Res <- pathPublisherAnnounceRes{Err: fmt.Errorf("terminated")}
	}
}

// OnPublisherRecord is called by a publisher.
func (pa *path) OnPublisherRecord(req pathPublisherRecordReq) pathPublisherRecordRes {
	req.Res = make(chan pathPublisherRecordRes)
	select {
	case pa.publisherRecord <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathPublisherRecordRes{Err: fmt.Errorf("terminated")}
	}
}

// OnPublisherPause is called by a publisher.
func (pa *path) OnPublisherPause(req pathPublisherPauseReq) {
	req.Res = make(chan struct{})
	select {
	case pa.publisherPause <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnReaderRemove is called by a reader.
func (pa *path) OnReaderRemove(req pathReaderRemoveReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerRemove <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnReaderSetupPlay is called by pathManager (asynchronous).
func (pa *path) OnReaderSetupPlay(req pathReaderSetupPlayReq) {
	select {
	case pa.readerSetupPlay <- req:
	case <-pa.ctx.Done():
		req.Res <- pathReaderSetupPlayRes{Err: fmt.Errorf("terminated")}
	}
}

// OnReaderPlay is called by a reader.
func (pa *path) OnReaderPlay(req pathReaderPlayReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerPlay <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnReaderPause is called by a reader.
func (pa *path) OnReaderPause(req pathReaderPauseReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerPause <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// OnSourceFrame is called by a source.
func (pa *path) OnSourceFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	// forward to RTSP readers
	pa.stream.WriteFrame(trackID, streamType, payload)

	// forward to non-RTSP readers
	pa.nonRTSPReaders.forwardFrame(trackID, streamType, payload)
}
