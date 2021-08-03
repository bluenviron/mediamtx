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

type pathOnDemandState int

const (
	pathOnDemandStateInitial pathOnDemandState = iota
	pathOnDemandStateWaitingReady
	pathOnDemandStateReady
	pathOnDemandStateClosing
)

type pathSourceStaticSetReadyReq struct {
	Tracks gortsplib.Tracks
	Res    chan struct{}
}

type pathSourceStaticSetNotReadyReq struct {
	Source sourceStatic
	Res    chan struct{}
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
	Path     *path
	Stream   *gortsplib.ServerStream
	Redirect string
	Err      error
}

type pathDescribeReq struct {
	PathName            string
	URL                 *base.URL
	IP                  net.IP
	ValidateCredentials func(pathUser string, pathPass string) error
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
	ValidateCredentials func(pathUser string, pathPass string) error
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
	ValidateCredentials func(pathUser string, pathPass string) error
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

	ctx                context.Context
	ctxCancel          func()
	source             source
	sourceReady        bool
	sourceStaticWg     sync.WaitGroup
	stream             *gortsplib.ServerStream
	readers            map[reader]pathReaderState
	describeRequests   []pathDescribeReq
	setupPlayRequests  []pathReaderSetupPlayReq
	nonRTSPReaders     *pathReadersMap
	onDemandCmd        *externalcmd.Cmd
	onPublishCmd       *externalcmd.Cmd
	onDemandReadyTimer *time.Timer
	onDemandCloseTimer *time.Timer
	onDemandState      pathOnDemandState

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
		onDemandReadyTimer:      newEmptyTimer(),
		onDemandCloseTimer:      newEmptyTimer(),
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
	} else if !pa.conf.SourceOnDemand && pa.hasStaticSource() {
		pa.staticSourceCreate()
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
		case <-pa.onDemandReadyTimer.C:
			for _, req := range pa.describeRequests {
				req.Res <- pathDescribeRes{Err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
			}
			pa.describeRequests = nil

			for _, req := range pa.setupPlayRequests {
				req.Res <- pathReaderSetupPlayRes{Err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
			}
			pa.setupPlayRequests = nil

			pa.onDemandCloseSource()

			if pa.conf.Regexp != nil {
				break outer
			}

		case <-pa.onDemandCloseTimer.C:
			pa.onDemandCloseSource()

			if pa.conf.Regexp != nil {
				break outer
			}

		case req := <-pa.sourceStaticSetReady:
			pa.stream = gortsplib.NewServerStream(req.Tracks)
			pa.sourceSetReady()
			close(req.Res)

		case req := <-pa.sourceStaticSetNotReady:
			if req.Source == pa.source {
				pa.sourceSetNotReady()
			}
			close(req.Res)

			if pa.source == nil && pa.conf.Regexp != nil {
				break outer
			}

		case req := <-pa.describe:
			pa.onDescribe(req)

		case req := <-pa.publisherRemove:
			pa.onPublisherRemove(req)

			if pa.source == nil && pa.conf.Regexp != nil {
				break outer
			}

		case req := <-pa.publisherAnnounce:
			pa.onPublisherAnnounce(req)

		case req := <-pa.publisherRecord:
			pa.onPublisherRecord(req)

		case req := <-pa.publisherPause:
			pa.onPublisherPause(req)

			if pa.source == nil && pa.conf.Regexp != nil {
				break outer
			}

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

	pa.onDemandReadyTimer.Stop()
	pa.onDemandCloseTimer.Stop()

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
			if pa.sourceReady {
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

func (pa *path) isOnDemand() bool {
	return (pa.hasStaticSource() && pa.conf.SourceOnDemand) || pa.conf.RunOnDemand != ""
}

func (pa *path) onDemandStartSource() {
	pa.onDemandReadyTimer.Stop()
	if pa.hasStaticSource() {
		pa.staticSourceCreate()
		pa.onDemandReadyTimer = time.NewTimer(pa.conf.SourceOnDemandStartTimeout)

	} else {
		pa.Log(logger.Info, "on demand command started")
		_, port, _ := net.SplitHostPort(pa.rtspAddress)
		pa.onDemandCmd = externalcmd.New(pa.conf.RunOnDemand, pa.conf.RunOnDemandRestart, externalcmd.Environment{
			Path: pa.name,
			Port: port,
		})
		pa.onDemandReadyTimer = time.NewTimer(pa.conf.RunOnDemandStartTimeout)
	}

	pa.onDemandState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandScheduleClose() {
	pa.onDemandCloseTimer.Stop()
	if pa.hasStaticSource() {
		pa.onDemandCloseTimer = time.NewTimer(pa.conf.SourceOnDemandCloseAfter)
	} else {
		pa.onDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
	}

	pa.onDemandState = pathOnDemandStateClosing
}

func (pa *path) onDemandCloseSource() {
	if pa.onDemandState == pathOnDemandStateClosing {
		pa.onDemandCloseTimer.Stop()
		pa.onDemandCloseTimer = newEmptyTimer()
	}

	// set state before doPublisherRemove()
	pa.onDemandState = pathOnDemandStateInitial

	if pa.hasStaticSource() {
		pa.staticSourceDelete()
	} else {
		pa.Log(logger.Info, "on demand command stopped")
		pa.onDemandCmd.Close()
		pa.onDemandCmd = nil

		if pa.source != nil {
			pa.source.(publisher).Close()
			pa.doPublisherRemove()
		}
	}
}

func (pa *path) sourceSetReady() {
	pa.sourceReady = true

	if pa.isOnDemand() {
		pa.onDemandReadyTimer.Stop()
		pa.onDemandReadyTimer = newEmptyTimer()

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

		if len(pa.readers) > 0 {
			pa.onDemandState = pathOnDemandStateReady
		} else {
			pa.onDemandScheduleClose()
		}
	}

	pa.parent.OnPathSourceReady(pa)
}

func (pa *path) sourceSetNotReady() {
	pa.sourceReady = false

	if pa.isOnDemand() && pa.onDemandState != pathOnDemandStateInitial {
		pa.onDemandCloseSource()
	}

	if pa.onPublishCmd != nil {
		pa.onPublishCmd.Close()
		pa.onPublishCmd = nil
	}

	for r := range pa.readers {
		pa.doReaderRemove(r)
		r.Close()
	}
}

func (pa *path) staticSourceCreate() {
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

func (pa *path) staticSourceDelete() {
	pa.sourceReady = false

	pa.source.(sourceStatic).Close()
	pa.source = nil

	pa.stream.Close()
	pa.stream = nil
}

func (pa *path) doReaderRemove(r reader) {
	state := pa.readers[r]

	if state == pathReaderStatePlay {
		atomic.AddInt64(pa.stats.CountReaders, -1)

		if _, ok := r.(pathRTSPSession); !ok {
			pa.nonRTSPReaders.remove(r)
		}
	}

	delete(pa.readers, r)
}

func (pa *path) doPublisherRemove() {
	if pa.sourceReady {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.sourceSetNotReady()
	}

	pa.source = nil
	pa.stream.Close()
	pa.stream = nil

	for r := range pa.readers {
		pa.doReaderRemove(r)
		r.Close()
	}
}

func (pa *path) onDescribe(req pathDescribeReq) {
	if _, ok := pa.source.(*sourceRedirect); ok {
		req.Res <- pathDescribeRes{
			Redirect: pa.conf.SourceRedirect,
		}
		return
	}

	if pa.sourceReady {
		req.Res <- pathDescribeRes{
			Stream: pa.stream,
		}
		return
	}

	if pa.isOnDemand() {
		if pa.onDemandState == pathOnDemandStateInitial {
			pa.onDemandStartSource()
		}
		pa.describeRequests = append(pa.describeRequests, req)
		return
	}

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
}

func (pa *path) onPublisherRemove(req pathPublisherRemoveReq) {
	if pa.source == req.Author {
		pa.doPublisherRemove()
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
		pa.source.(publisher).Close()
		pa.doPublisherRemove()
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

	pa.sourceSetReady()

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
	if req.Author == pa.source && pa.sourceReady {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.sourceSetNotReady()
	}
	close(req.Res)
}

func (pa *path) onReaderRemove(req pathReaderRemoveReq) {
	if _, ok := pa.readers[req.Author]; ok {
		pa.doReaderRemove(req.Author)
	}
	close(req.Res)

	if pa.isOnDemand() &&
		len(pa.readers) == 0 &&
		pa.onDemandState == pathOnDemandStateReady {
		pa.onDemandScheduleClose()
	}
}

func (pa *path) onReaderSetupPlay(req pathReaderSetupPlayReq) {
	if pa.sourceReady {
		pa.onReaderSetupPlayPost(req)
		return
	}

	if pa.isOnDemand() {
		if pa.onDemandState == pathOnDemandStateInitial {
			pa.onDemandStartSource()
		}
		pa.setupPlayRequests = append(pa.setupPlayRequests, req)
		return
	}

	req.Res <- pathReaderSetupPlayRes{Err: pathErrNoOnePublishing{PathName: pa.name}}
}

func (pa *path) onReaderSetupPlayPost(req pathReaderSetupPlayReq) {
	pa.readers[req.Author] = pathReaderStatePrePlay

	if pa.isOnDemand() && pa.onDemandState == pathOnDemandStateClosing {
		pa.onDemandState = pathOnDemandStateReady
		pa.onDemandCloseTimer.Stop()
		pa.onDemandCloseTimer = newEmptyTimer()
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

// OnDescribe is called by a reader or publisher through pathManager.
func (pa *path) OnDescribe(req pathDescribeReq) pathDescribeRes {
	select {
	case pa.describe <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathDescribeRes{Err: fmt.Errorf("terminated")}
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

// OnPublisherAnnounce is called by a publisher through pathManager.
func (pa *path) OnPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	select {
	case pa.publisherAnnounce <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathPublisherAnnounceRes{Err: fmt.Errorf("terminated")}
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

// OnReaderSetupPlay is called by a reader through pathManager.
func (pa *path) OnReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	select {
	case pa.readerSetupPlay <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathReaderSetupPlayRes{Err: fmt.Errorf("terminated")}
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
