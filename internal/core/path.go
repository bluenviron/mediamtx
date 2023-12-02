package core

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/record"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func newEmptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type errPathNoOnePublishing struct {
	pathName string
}

// Error implements the error interface.
func (e errPathNoOnePublishing) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.pathName)
}

type pathParent interface {
	logger.Writer
	pathReady(*path)
	pathNotReady(*path)
	closePath(*path)
}

type pathOnDemandState int

const (
	pathOnDemandStateInitial pathOnDemandState = iota
	pathOnDemandStateWaitingReady
	pathOnDemandStateReady
	pathOnDemandStateClosing
)

type pathAccessRequest struct {
	name     string
	query    string
	publish  bool
	skipAuth bool

	// only if skipAuth = false
	ip          net.IP
	user        string
	pass        string
	proto       authProtocol
	id          *uuid.UUID
	rtspRequest *base.Request
	rtspBaseURL *base.URL
	rtspNonce   string
}

type pathRemoveReaderReq struct {
	author reader
	res    chan struct{}
}

type pathRemovePublisherReq struct {
	author publisher
	res    chan struct{}
}

type pathGetConfForPathRes struct {
	conf *conf.Path
	err  error
}

type pathGetConfForPathReq struct {
	accessRequest pathAccessRequest
	res           chan pathGetConfForPathRes
}

type pathDescribeRes struct {
	path     *path
	stream   *stream.Stream
	redirect string
	err      error
}

type pathDescribeReq struct {
	accessRequest pathAccessRequest
	res           chan pathDescribeRes
}

type pathAddReaderRes struct {
	path   *path
	stream *stream.Stream
	err    error
}

type pathAddReaderReq struct {
	author        reader
	accessRequest pathAccessRequest
	res           chan pathAddReaderRes
}

type pathAddPublisherRes struct {
	path *path
	err  error
}

type pathAddPublisherReq struct {
	author        publisher
	accessRequest pathAccessRequest
	res           chan pathAddPublisherRes
}

type pathStartPublisherRes struct {
	stream *stream.Stream
	err    error
}

type pathStartPublisherReq struct {
	author             publisher
	desc               *description.Session
	generateRTPPackets bool
	res                chan pathStartPublisherRes
}

type pathStopPublisherReq struct {
	author publisher
	res    chan struct{}
}

type pathAPIPathsListRes struct {
	data  *defs.APIPathList
	paths map[string]*path
}

type pathAPIPathsListReq struct {
	res chan pathAPIPathsListRes
}

type pathAPIPathsGetRes struct {
	path *path
	data *defs.APIPath
	err  error
}

type pathAPIPathsGetReq struct {
	name string
	res  chan pathAPIPathsGetRes
}

type path struct {
	rtspAddress       string
	readTimeout       conf.StringDuration
	writeTimeout      conf.StringDuration
	writeQueueSize    int
	udpMaxPayloadSize int
	confName          string
	conf              *conf.Path
	name              string
	matches           []string
	wg                *sync.WaitGroup
	externalCmdPool   *externalcmd.Pool
	parent            pathParent

	ctx                            context.Context
	ctxCancel                      func()
	confMutex                      sync.RWMutex
	source                         source
	publisherQuery                 string
	stream                         *stream.Stream
	recordAgent                    *record.Agent
	readyTime                      time.Time
	onUnDemandHook                 func(string)
	onNotReadyHook                 func()
	readers                        map[reader]struct{}
	describeRequestsOnHold         []pathDescribeReq
	readerAddRequestsOnHold        []pathAddReaderReq
	onDemandStaticSourceState      pathOnDemandState
	onDemandStaticSourceReadyTimer *time.Timer
	onDemandStaticSourceCloseTimer *time.Timer
	onDemandPublisherState         pathOnDemandState
	onDemandPublisherReadyTimer    *time.Timer
	onDemandPublisherCloseTimer    *time.Timer

	// in
	chReloadConf              chan *conf.Path
	chStaticSourceSetReady    chan defs.PathSourceStaticSetReadyReq
	chStaticSourceSetNotReady chan defs.PathSourceStaticSetNotReadyReq
	chDescribe                chan pathDescribeReq
	chAddPublisher            chan pathAddPublisherReq
	chRemovePublisher         chan pathRemovePublisherReq
	chStartPublisher          chan pathStartPublisherReq
	chStopPublisher           chan pathStopPublisherReq
	chAddReader               chan pathAddReaderReq
	chRemoveReader            chan pathRemoveReaderReq
	chAPIPathsGet             chan pathAPIPathsGetReq

	// out
	done chan struct{}
}

func newPath(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	writeQueueSize int,
	udpMaxPayloadSize int,
	confName string,
	cnf *conf.Path,
	name string,
	matches []string,
	wg *sync.WaitGroup,
	externalCmdPool *externalcmd.Pool,
	parent pathParent,
) *path {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	pa := &path{
		rtspAddress:                    rtspAddress,
		readTimeout:                    readTimeout,
		writeTimeout:                   writeTimeout,
		writeQueueSize:                 writeQueueSize,
		udpMaxPayloadSize:              udpMaxPayloadSize,
		confName:                       confName,
		conf:                           cnf,
		name:                           name,
		matches:                        matches,
		wg:                             wg,
		externalCmdPool:                externalCmdPool,
		parent:                         parent,
		ctx:                            ctx,
		ctxCancel:                      ctxCancel,
		readers:                        make(map[reader]struct{}),
		onDemandStaticSourceReadyTimer: newEmptyTimer(),
		onDemandStaticSourceCloseTimer: newEmptyTimer(),
		onDemandPublisherReadyTimer:    newEmptyTimer(),
		onDemandPublisherCloseTimer:    newEmptyTimer(),
		chReloadConf:                   make(chan *conf.Path),
		chStaticSourceSetReady:         make(chan defs.PathSourceStaticSetReadyReq),
		chStaticSourceSetNotReady:      make(chan defs.PathSourceStaticSetNotReadyReq),
		chDescribe:                     make(chan pathDescribeReq),
		chAddPublisher:                 make(chan pathAddPublisherReq),
		chRemovePublisher:              make(chan pathRemovePublisherReq),
		chStartPublisher:               make(chan pathStartPublisherReq),
		chStopPublisher:                make(chan pathStopPublisherReq),
		chAddReader:                    make(chan pathAddReaderReq),
		chRemoveReader:                 make(chan pathRemoveReaderReq),
		chAPIPathsGet:                  make(chan pathAPIPathsGetReq),
		done:                           make(chan struct{}),
	}

	pa.Log(logger.Debug, "created")

	pa.wg.Add(1)
	go pa.run()

	return pa
}

func (pa *path) close() {
	pa.ctxCancel()
}

func (pa *path) wait() {
	<-pa.done
}

// Log is the main logging function.
func (pa *path) Log(level logger.Level, format string, args ...interface{}) {
	pa.parent.Log(level, "[path "+pa.name+"] "+format, args...)
}

func (pa *path) run() {
	defer close(pa.done)
	defer pa.wg.Done()

	if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	} else if pa.conf.HasStaticSource() {
		pa.source = newStaticSourceHandler(
			pa.conf,
			pa.readTimeout,
			pa.writeTimeout,
			pa.writeQueueSize,
			pa)

		if !pa.conf.SourceOnDemand {
			pa.source.(*staticSourceHandler).start(false)
		}
	}

	onUnInitHook := hooks.OnInit(hooks.OnInitParams{
		Logger:          pa,
		ExternalCmdPool: pa.externalCmdPool,
		Conf:            pa.conf,
		ExternalCmdEnv:  pa.externalCmdEnv(),
	})

	err := pa.runInner()

	// call before destroying context
	pa.parent.closePath(pa)

	pa.ctxCancel()

	pa.onDemandStaticSourceReadyTimer.Stop()
	pa.onDemandStaticSourceCloseTimer.Stop()
	pa.onDemandPublisherReadyTimer.Stop()
	pa.onDemandPublisherCloseTimer.Stop()

	onUnInitHook()

	for _, req := range pa.describeRequestsOnHold {
		req.res <- pathDescribeRes{err: fmt.Errorf("terminated")}
	}

	for _, req := range pa.readerAddRequestsOnHold {
		req.res <- pathAddReaderRes{err: fmt.Errorf("terminated")}
	}

	if pa.stream != nil {
		pa.setNotReady()
	}

	if pa.source != nil {
		if source, ok := pa.source.(*staticSourceHandler); ok {
			if !pa.conf.SourceOnDemand || pa.onDemandStaticSourceState != pathOnDemandStateInitial {
				source.close("path is closing")
			}
		} else if source, ok := pa.source.(publisher); ok {
			source.close()
		}
	}

	if pa.onUnDemandHook != nil {
		pa.onUnDemandHook("path destroyed")
	}

	pa.Log(logger.Debug, "destroyed: %v", err)
}

func (pa *path) runInner() error {
	for {
		select {
		case <-pa.onDemandStaticSourceReadyTimer.C:
			pa.doOnDemandStaticSourceReadyTimer()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandStaticSourceCloseTimer.C:
			pa.doOnDemandStaticSourceCloseTimer()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandPublisherReadyTimer.C:
			pa.doOnDemandPublisherReadyTimer()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandPublisherCloseTimer.C:
			pa.doOnDemandPublisherCloseTimer()

		case newConf := <-pa.chReloadConf:
			pa.doReloadConf(newConf)

		case req := <-pa.chStaticSourceSetReady:
			pa.doSourceStaticSetReady(req)

		case req := <-pa.chStaticSourceSetNotReady:
			pa.doSourceStaticSetNotReady(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chDescribe:
			pa.doDescribe(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chAddPublisher:
			pa.doAddPublisher(req)

		case req := <-pa.chRemovePublisher:
			pa.doRemovePublisher(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chStartPublisher:
			pa.doStartPublisher(req)

		case req := <-pa.chStopPublisher:
			pa.doStopPublisher(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chAddReader:
			pa.doAddReader(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chRemoveReader:
			pa.doRemoveReader(req)

		case req := <-pa.chAPIPathsGet:
			pa.doAPIPathsGet(req)

		case <-pa.ctx.Done():
			return fmt.Errorf("terminated")
		}
	}
}

func (pa *path) doOnDemandStaticSourceReadyTimer() {
	for _, req := range pa.describeRequestsOnHold {
		req.res <- pathDescribeRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.describeRequestsOnHold = nil

	for _, req := range pa.readerAddRequestsOnHold {
		req.res <- pathAddReaderRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.readerAddRequestsOnHold = nil

	pa.onDemandStaticSourceStop("timed out")
}

func (pa *path) doOnDemandStaticSourceCloseTimer() {
	pa.setNotReady()
	pa.onDemandStaticSourceStop("not needed by anyone")
}

func (pa *path) doOnDemandPublisherReadyTimer() {
	for _, req := range pa.describeRequestsOnHold {
		req.res <- pathDescribeRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.describeRequestsOnHold = nil

	for _, req := range pa.readerAddRequestsOnHold {
		req.res <- pathAddReaderRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
	}
	pa.readerAddRequestsOnHold = nil

	pa.onDemandPublisherStop("timed out")
}

func (pa *path) doOnDemandPublisherCloseTimer() {
	pa.onDemandPublisherStop("not needed by anyone")
}

func (pa *path) doReloadConf(newConf *conf.Path) {
	pa.confMutex.Lock()
	pa.conf = newConf
	pa.confMutex.Unlock()

	if pa.conf.HasStaticSource() {
		go pa.source.(*staticSourceHandler).reloadConf(newConf)
	}

	if pa.conf.Record {
		if pa.stream != nil && pa.recordAgent == nil {
			pa.startRecording()
		}
	} else if pa.recordAgent != nil {
		pa.recordAgent.Close()
		pa.recordAgent = nil
	}
}

func (pa *path) doSourceStaticSetReady(req defs.PathSourceStaticSetReadyReq) {
	err := pa.setReady(req.Desc, req.GenerateRTPPackets)
	if err != nil {
		req.Res <- defs.PathSourceStaticSetReadyRes{Err: err}
		return
	}

	if pa.conf.HasOnDemandStaticSource() {
		pa.onDemandStaticSourceReadyTimer.Stop()
		pa.onDemandStaticSourceReadyTimer = newEmptyTimer()
		pa.onDemandStaticSourceScheduleClose()
	}

	pa.consumeOnHoldRequests()

	req.Res <- defs.PathSourceStaticSetReadyRes{Stream: pa.stream}
}

func (pa *path) doSourceStaticSetNotReady(req defs.PathSourceStaticSetNotReadyReq) {
	pa.setNotReady()

	// send response before calling onDemandStaticSourceStop()
	// in order to avoid a deadlock due to staticSourceHandler.stop()
	close(req.Res)

	if pa.conf.HasOnDemandStaticSource() && pa.onDemandStaticSourceState != pathOnDemandStateInitial {
		pa.onDemandStaticSourceStop("an error occurred")
	}
}

func (pa *path) doDescribe(req pathDescribeReq) {
	if _, ok := pa.source.(*sourceRedirect); ok {
		req.res <- pathDescribeRes{
			redirect: pa.conf.SourceRedirect,
		}
		return
	}

	if pa.stream != nil {
		req.res <- pathDescribeRes{
			stream: pa.stream,
		}
		return
	}

	if pa.conf.HasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateInitial {
			pa.onDemandStaticSourceStart()
		}
		pa.describeRequestsOnHold = append(pa.describeRequestsOnHold, req)
		return
	}

	if pa.conf.HasOnDemandPublisher() {
		if pa.onDemandPublisherState == pathOnDemandStateInitial {
			pa.onDemandPublisherStart(req.accessRequest.query)
		}
		pa.describeRequestsOnHold = append(pa.describeRequestsOnHold, req)
		return
	}

	if pa.conf.Fallback != "" {
		fallbackURL := func() string {
			if strings.HasPrefix(pa.conf.Fallback, "/") {
				ur := base.URL{
					Scheme: req.accessRequest.rtspRequest.URL.Scheme,
					User:   req.accessRequest.rtspRequest.URL.User,
					Host:   req.accessRequest.rtspRequest.URL.Host,
					Path:   pa.conf.Fallback,
				}
				return ur.String()
			}
			return pa.conf.Fallback
		}()
		req.res <- pathDescribeRes{redirect: fallbackURL}
		return
	}

	req.res <- pathDescribeRes{err: errPathNoOnePublishing{pathName: pa.name}}
}

func (pa *path) doRemovePublisher(req pathRemovePublisherReq) {
	if pa.source == req.author {
		pa.executeRemovePublisher()
	}
	close(req.res)
}

func (pa *path) doAddPublisher(req pathAddPublisherReq) {
	if pa.conf.Source != "publisher" {
		req.res <- pathAddPublisherRes{
			err: fmt.Errorf("can't publish to path '%s' since 'source' is not 'publisher'", pa.name),
		}
		return
	}

	if pa.source != nil {
		if !pa.conf.OverridePublisher {
			req.res <- pathAddPublisherRes{err: fmt.Errorf("someone is already publishing to path '%s'", pa.name)}
			return
		}

		pa.Log(logger.Info, "closing existing publisher")
		pa.source.(publisher).close()
		pa.executeRemovePublisher()
	}

	pa.source = req.author
	pa.publisherQuery = req.accessRequest.query

	req.res <- pathAddPublisherRes{path: pa}
}

func (pa *path) doStartPublisher(req pathStartPublisherReq) {
	if pa.source != req.author {
		req.res <- pathStartPublisherRes{err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	err := pa.setReady(req.desc, req.generateRTPPackets)
	if err != nil {
		req.res <- pathStartPublisherRes{err: err}
		return
	}

	req.author.Log(logger.Info, "is publishing to path '%s', %s",
		pa.name,
		mediaInfo(req.desc.Medias))

	if pa.conf.HasOnDemandPublisher() && pa.onDemandPublisherState != pathOnDemandStateInitial {
		pa.onDemandPublisherReadyTimer.Stop()
		pa.onDemandPublisherReadyTimer = newEmptyTimer()
		pa.onDemandPublisherScheduleClose()
	}

	pa.consumeOnHoldRequests()

	req.res <- pathStartPublisherRes{stream: pa.stream}
}

func (pa *path) doStopPublisher(req pathStopPublisherReq) {
	if req.author == pa.source && pa.stream != nil {
		pa.setNotReady()
	}
	close(req.res)
}

func (pa *path) doAddReader(req pathAddReaderReq) {
	if pa.stream != nil {
		pa.addReaderPost(req)
		return
	}

	if pa.conf.HasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateInitial {
			pa.onDemandStaticSourceStart()
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	if pa.conf.HasOnDemandPublisher() {
		if pa.onDemandPublisherState == pathOnDemandStateInitial {
			pa.onDemandPublisherStart(req.accessRequest.query)
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	req.res <- pathAddReaderRes{err: errPathNoOnePublishing{pathName: pa.name}}
}

func (pa *path) doRemoveReader(req pathRemoveReaderReq) {
	if _, ok := pa.readers[req.author]; ok {
		pa.executeRemoveReader(req.author)
	}
	close(req.res)

	if len(pa.readers) == 0 {
		if pa.conf.HasOnDemandStaticSource() {
			if pa.onDemandStaticSourceState == pathOnDemandStateReady {
				pa.onDemandStaticSourceScheduleClose()
			}
		} else if pa.conf.HasOnDemandPublisher() {
			if pa.onDemandPublisherState == pathOnDemandStateReady {
				pa.onDemandPublisherScheduleClose()
			}
		}
	}
}

func (pa *path) doAPIPathsGet(req pathAPIPathsGetReq) {
	req.res <- pathAPIPathsGetRes{
		data: &defs.APIPath{
			Name:     pa.name,
			ConfName: pa.confName,
			Source: func() *defs.APIPathSourceOrReader {
				if pa.source == nil {
					return nil
				}
				v := pa.source.APISourceDescribe()
				return &v
			}(),
			Ready: pa.stream != nil,
			ReadyTime: func() *time.Time {
				if pa.stream == nil {
					return nil
				}
				v := pa.readyTime
				return &v
			}(),
			Tracks: func() []string {
				if pa.stream == nil {
					return []string{}
				}
				return mediasDescription(pa.stream.Desc().Medias)
			}(),
			BytesReceived: func() uint64 {
				if pa.stream == nil {
					return 0
				}
				return pa.stream.BytesReceived()
			}(),
			BytesSent: func() uint64 {
				if pa.stream == nil {
					return 0
				}
				return pa.stream.BytesSent()
			}(),
			Readers: func() []defs.APIPathSourceOrReader {
				ret := []defs.APIPathSourceOrReader{}
				for r := range pa.readers {
					ret = append(ret, r.apiReaderDescribe())
				}
				return ret
			}(),
		},
	}
}

func (pa *path) safeConf() *conf.Path {
	pa.confMutex.RLock()
	defer pa.confMutex.RUnlock()
	return pa.conf
}

func (pa *path) shouldClose() bool {
	return pa.conf.Regexp != nil &&
		pa.source == nil &&
		len(pa.readers) == 0 &&
		len(pa.describeRequestsOnHold) == 0 &&
		len(pa.readerAddRequestsOnHold) == 0
}

func (pa *path) externalCmdEnv() externalcmd.Environment {
	_, port, _ := net.SplitHostPort(pa.rtspAddress)
	env := externalcmd.Environment{
		"MTX_PATH":  pa.name,
		"RTSP_PATH": pa.name, // deprecated
		"RTSP_PORT": port,
	}

	if len(pa.matches) > 1 {
		for i, ma := range pa.matches[1:] {
			env["G"+strconv.FormatInt(int64(i+1), 10)] = ma
		}
	}

	return env
}

func (pa *path) onDemandStaticSourceStart() {
	pa.source.(*staticSourceHandler).start(true)

	pa.onDemandStaticSourceReadyTimer.Stop()
	pa.onDemandStaticSourceReadyTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandStartTimeout))

	pa.onDemandStaticSourceState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandStaticSourceScheduleClose() {
	pa.onDemandStaticSourceCloseTimer.Stop()
	pa.onDemandStaticSourceCloseTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandCloseAfter))

	pa.onDemandStaticSourceState = pathOnDemandStateClosing
}

func (pa *path) onDemandStaticSourceStop(reason string) {
	if pa.onDemandStaticSourceState == pathOnDemandStateClosing {
		pa.onDemandStaticSourceCloseTimer.Stop()
		pa.onDemandStaticSourceCloseTimer = newEmptyTimer()
	}

	pa.onDemandStaticSourceState = pathOnDemandStateInitial

	pa.source.(*staticSourceHandler).stop(reason)
}

func (pa *path) onDemandPublisherStart(query string) {
	pa.onUnDemandHook = hooks.OnDemand(hooks.OnDemandParams{
		Logger:          pa,
		ExternalCmdPool: pa.externalCmdPool,
		Conf:            pa.conf,
		ExternalCmdEnv:  pa.externalCmdEnv(),
		Query:           query,
	})

	pa.onDemandPublisherReadyTimer.Stop()
	pa.onDemandPublisherReadyTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandStartTimeout))

	pa.onDemandPublisherState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandPublisherScheduleClose() {
	pa.onDemandPublisherCloseTimer.Stop()
	pa.onDemandPublisherCloseTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandCloseAfter))

	pa.onDemandPublisherState = pathOnDemandStateClosing
}

func (pa *path) onDemandPublisherStop(reason string) {
	if pa.onDemandPublisherState == pathOnDemandStateClosing {
		pa.onDemandPublisherCloseTimer.Stop()
		pa.onDemandPublisherCloseTimer = newEmptyTimer()
	}

	pa.onUnDemandHook(reason)
	pa.onUnDemandHook = nil

	pa.onDemandPublisherState = pathOnDemandStateInitial
}

func (pa *path) setReady(desc *description.Session, allocateEncoder bool) error {
	var err error
	pa.stream, err = stream.New(
		pa.udpMaxPayloadSize,
		desc,
		allocateEncoder,
		logger.NewLimitedLogger(pa.source),
	)
	if err != nil {
		return err
	}

	if pa.conf.Record {
		pa.startRecording()
	}

	pa.readyTime = time.Now()

	pa.onNotReadyHook = hooks.OnReady(hooks.OnReadyParams{
		Logger:          pa,
		ExternalCmdPool: pa.externalCmdPool,
		Conf:            pa.conf,
		ExternalCmdEnv:  pa.externalCmdEnv(),
		Desc:            pa.source.APISourceDescribe(),
		Query:           pa.publisherQuery,
	})

	pa.parent.pathReady(pa)

	return nil
}

func (pa *path) consumeOnHoldRequests() {
	for _, req := range pa.describeRequestsOnHold {
		req.res <- pathDescribeRes{
			stream: pa.stream,
		}
	}
	pa.describeRequestsOnHold = nil

	for _, req := range pa.readerAddRequestsOnHold {
		pa.addReaderPost(req)
	}
	pa.readerAddRequestsOnHold = nil
}

func (pa *path) setNotReady() {
	pa.parent.pathNotReady(pa)

	for r := range pa.readers {
		pa.executeRemoveReader(r)
		r.close()
	}

	pa.onNotReadyHook()

	if pa.recordAgent != nil {
		pa.recordAgent.Close()
		pa.recordAgent = nil
	}

	if pa.stream != nil {
		pa.stream.Close()
		pa.stream = nil
	}
}

func (pa *path) startRecording() {
	pa.recordAgent = &record.Agent{
		WriteQueueSize:  pa.writeQueueSize,
		PathFormat:      pa.conf.RecordPath,
		Format:          pa.conf.RecordFormat,
		PartDuration:    time.Duration(pa.conf.RecordPartDuration),
		SegmentDuration: time.Duration(pa.conf.RecordSegmentDuration),
		PathName:        pa.name,
		Stream:          pa.stream,
		OnSegmentCreate: func(path string) {
			if pa.conf.RunOnRecordSegmentCreate != "" {
				env := pa.externalCmdEnv()
				env["MTX_SEGMENT_PATH"] = path

				pa.Log(logger.Info, "runOnRecordSegmentCreate command launched")
				externalcmd.NewCmd(
					pa.externalCmdPool,
					pa.conf.RunOnRecordSegmentCreate,
					false,
					env,
					nil)
			}
		},
		OnSegmentComplete: func(path string) {
			if pa.conf.RunOnRecordSegmentComplete != "" {
				env := pa.externalCmdEnv()
				env["MTX_SEGMENT_PATH"] = path

				pa.Log(logger.Info, "runOnRecordSegmentComplete command launched")
				externalcmd.NewCmd(
					pa.externalCmdPool,
					pa.conf.RunOnRecordSegmentComplete,
					false,
					env,
					nil)
			}
		},
		Parent: pa,
	}
	pa.recordAgent.Initialize()
}

func (pa *path) executeRemoveReader(r reader) {
	delete(pa.readers, r)
}

func (pa *path) executeRemovePublisher() {
	if pa.stream != nil {
		pa.setNotReady()
	}

	pa.source = nil
}

func (pa *path) addReaderPost(req pathAddReaderReq) {
	if _, ok := pa.readers[req.author]; ok {
		req.res <- pathAddReaderRes{
			path:   pa,
			stream: pa.stream,
		}
		return
	}

	if pa.conf.MaxReaders != 0 && len(pa.readers) >= pa.conf.MaxReaders {
		req.res <- pathAddReaderRes{
			err: fmt.Errorf("maximum reader count reached"),
		}
		return
	}

	pa.readers[req.author] = struct{}{}

	if pa.conf.HasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateClosing {
			pa.onDemandStaticSourceState = pathOnDemandStateReady
			pa.onDemandStaticSourceCloseTimer.Stop()
			pa.onDemandStaticSourceCloseTimer = newEmptyTimer()
		}
	} else if pa.conf.HasOnDemandPublisher() {
		if pa.onDemandPublisherState == pathOnDemandStateClosing {
			pa.onDemandPublisherState = pathOnDemandStateReady
			pa.onDemandPublisherCloseTimer.Stop()
			pa.onDemandPublisherCloseTimer = newEmptyTimer()
		}
	}

	req.res <- pathAddReaderRes{
		path:   pa,
		stream: pa.stream,
	}
}

// reloadConf is called by pathManager.
func (pa *path) reloadConf(newConf *conf.Path) {
	select {
	case pa.chReloadConf <- newConf:
	case <-pa.ctx.Done():
	}
}

// staticSourceHandlerSetReady is called by staticSourceHandler.
func (pa *path) staticSourceHandlerSetReady(
	staticSourceHandlerCtx context.Context, req defs.PathSourceStaticSetReadyReq,
) {
	select {
	case pa.chStaticSourceSetReady <- req:

	case <-pa.ctx.Done():
		req.Res <- defs.PathSourceStaticSetReadyRes{Err: fmt.Errorf("terminated")}

	// this avoids:
	// - invalid requests sent after the source has been terminated
	// - deadlocks caused by <-done inside stop()
	case <-staticSourceHandlerCtx.Done():
		req.Res <- defs.PathSourceStaticSetReadyRes{Err: fmt.Errorf("terminated")}
	}
}

// staticSourceHandlerSetNotReady is called by staticSourceHandler.
func (pa *path) staticSourceHandlerSetNotReady(
	staticSourceHandlerCtx context.Context, req defs.PathSourceStaticSetNotReadyReq,
) {
	select {
	case pa.chStaticSourceSetNotReady <- req:

	case <-pa.ctx.Done():
		close(req.Res)

	// this avoids:
	// - invalid requests sent after the source has been terminated
	// - deadlocks caused by <-done inside stop()
	case <-staticSourceHandlerCtx.Done():
		close(req.Res)
	}
}

// describe is called by a reader or publisher through pathManager.
func (pa *path) describe(req pathDescribeReq) pathDescribeRes {
	select {
	case pa.chDescribe <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathDescribeRes{err: fmt.Errorf("terminated")}
	}
}

// addPublisher is called by a publisher through pathManager.
func (pa *path) addPublisher(req pathAddPublisherReq) pathAddPublisherRes {
	select {
	case pa.chAddPublisher <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathAddPublisherRes{err: fmt.Errorf("terminated")}
	}
}

// removePublisher is called by a publisher.
func (pa *path) removePublisher(req pathRemovePublisherReq) {
	req.res = make(chan struct{})
	select {
	case pa.chRemovePublisher <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// startPublisher is called by a publisher.
func (pa *path) startPublisher(req pathStartPublisherReq) pathStartPublisherRes {
	req.res = make(chan pathStartPublisherRes)
	select {
	case pa.chStartPublisher <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathStartPublisherRes{err: fmt.Errorf("terminated")}
	}
}

// stopPublisher is called by a publisher.
func (pa *path) stopPublisher(req pathStopPublisherReq) {
	req.res = make(chan struct{})
	select {
	case pa.chStopPublisher <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// addReader is called by a reader through pathManager.
func (pa *path) addReader(req pathAddReaderReq) pathAddReaderRes {
	select {
	case pa.chAddReader <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathAddReaderRes{err: fmt.Errorf("terminated")}
	}
}

// removeReader is called by a reader.
func (pa *path) removeReader(req pathRemoveReaderReq) {
	req.res = make(chan struct{})
	select {
	case pa.chRemoveReader <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// apiPathsGet is called by api.
func (pa *path) apiPathsGet(req pathAPIPathsGetReq) (*defs.APIPath, error) {
	req.res = make(chan pathAPIPathsGetRes)
	select {
	case pa.chAPIPathsGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-pa.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}
