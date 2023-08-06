package core

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/gortsplib/v3/pkg/url"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
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

type pathSourceStaticSetReadyRes struct {
	stream *stream.Stream
	err    error
}

type pathSourceStaticSetReadyReq struct {
	medias             media.Medias
	generateRTPPackets bool
	res                chan pathSourceStaticSetReadyRes
}

type pathSourceStaticSetNotReadyReq struct {
	res chan struct{}
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
	conf *conf.PathConf
	err  error
}

type pathGetConfForPathReq struct {
	name        string
	publish     bool
	credentials authCredentials
	res         chan pathGetConfForPathRes
}

type pathDescribeRes struct {
	path     *path
	stream   *stream.Stream
	redirect string
	err      error
}

type pathDescribeReq struct {
	pathName    string
	url         *url.URL
	credentials authCredentials
	res         chan pathDescribeRes
}

type pathAddReaderRes struct {
	path   *path
	stream *stream.Stream
	err    error
}

type pathAddReaderReq struct {
	author      reader
	pathName    string
	skipAuth    bool
	credentials authCredentials
	res         chan pathAddReaderRes
}

type pathAddPublisherRes struct {
	path *path
	err  error
}

type pathAddPublisherReq struct {
	author      publisher
	pathName    string
	skipAuth    bool
	credentials authCredentials
	res         chan pathAddPublisherRes
}

type pathStartPublisherRes struct {
	stream *stream.Stream
	err    error
}

type pathStartPublisherReq struct {
	author             publisher
	medias             media.Medias
	generateRTPPackets bool
	res                chan pathStartPublisherRes
}

type pathStopPublisherReq struct {
	author publisher
	res    chan struct{}
}

type pathAPISourceOrReader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type pathAPIPathsListRes struct {
	data  *apiPathsList
	paths map[string]*path
}

type pathAPIPathsListReq struct {
	res chan pathAPIPathsListRes
}

type pathAPIPathsGetRes struct {
	path *path
	data *apiPath
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
	readBufferCount   int
	udpMaxPayloadSize int
	confName          string
	conf              *conf.PathConf
	name              string
	matches           []string
	wg                *sync.WaitGroup
	externalCmdPool   *externalcmd.Pool
	parent            pathParent

	ctx                            context.Context
	ctxCancel                      func()
	confMutex                      sync.RWMutex
	source                         source
	stream                         *stream.Stream
	readyTime                      time.Time
	bytesReceived                  *uint64
	readers                        map[reader]struct{}
	describeRequestsOnHold         []pathDescribeReq
	readerAddRequestsOnHold        []pathAddReaderReq
	onDemandCmd                    *externalcmd.Cmd
	onReadyCmd                     *externalcmd.Cmd
	onDemandStaticSourceState      pathOnDemandState
	onDemandStaticSourceReadyTimer *time.Timer
	onDemandStaticSourceCloseTimer *time.Timer
	onDemandPublisherState         pathOnDemandState
	onDemandPublisherReadyTimer    *time.Timer
	onDemandPublisherCloseTimer    *time.Timer

	// in
	chReloadConf              chan *conf.PathConf
	chSourceStaticSetReady    chan pathSourceStaticSetReadyReq
	chSourceStaticSetNotReady chan pathSourceStaticSetNotReadyReq
	chDescribe                chan pathDescribeReq
	chRemovePublisher         chan pathRemovePublisherReq
	chAddPublisher            chan pathAddPublisherReq
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
	readBufferCount int,
	udpMaxPayloadSize int,
	confName string,
	cnf *conf.PathConf,
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
		readBufferCount:                readBufferCount,
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
		bytesReceived:                  new(uint64),
		readers:                        make(map[reader]struct{}),
		onDemandStaticSourceReadyTimer: newEmptyTimer(),
		onDemandStaticSourceCloseTimer: newEmptyTimer(),
		onDemandPublisherReadyTimer:    newEmptyTimer(),
		onDemandPublisherCloseTimer:    newEmptyTimer(),
		chReloadConf:                   make(chan *conf.PathConf),
		chSourceStaticSetReady:         make(chan pathSourceStaticSetReadyReq),
		chSourceStaticSetNotReady:      make(chan pathSourceStaticSetNotReadyReq),
		chDescribe:                     make(chan pathDescribeReq),
		chRemovePublisher:              make(chan pathRemovePublisherReq),
		chAddPublisher:                 make(chan pathAddPublisherReq),
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

func (pa *path) safeConf() *conf.PathConf {
	pa.confMutex.RLock()
	defer pa.confMutex.RUnlock()
	return pa.conf
}

func (pa *path) run() {
	defer close(pa.done)
	defer pa.wg.Done()

	if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	} else if pa.conf.HasStaticSource() {
		pa.source = newSourceStatic(
			pa.conf,
			pa.readTimeout,
			pa.writeTimeout,
			pa.readBufferCount,
			pa)

		if !pa.conf.SourceOnDemand {
			pa.source.(*sourceStatic).start()
		}
	}

	var onInitCmd *externalcmd.Cmd
	if pa.conf.RunOnInit != "" {
		pa.Log(logger.Info, "runOnInit command started")
		onInitCmd = externalcmd.NewCmd(
			pa.externalCmdPool,
			pa.conf.RunOnInit,
			pa.conf.RunOnInitRestart,
			pa.externalCmdEnv(),
			func(err error) {
				pa.Log(logger.Info, "runOnInit command exited: %v", err)
			})
	}

	err := pa.runInner()

	// call before destroying context
	pa.parent.closePath(pa)

	pa.ctxCancel()

	pa.onDemandStaticSourceReadyTimer.Stop()
	pa.onDemandStaticSourceCloseTimer.Stop()
	pa.onDemandPublisherReadyTimer.Stop()
	pa.onDemandPublisherCloseTimer.Stop()

	if onInitCmd != nil {
		onInitCmd.Close()
		pa.Log(logger.Info, "runOnInit command stopped")
	}

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
		if source, ok := pa.source.(*sourceStatic); ok {
			source.close()
		} else if source, ok := pa.source.(publisher); ok {
			source.close()
		}
	}

	if pa.onDemandCmd != nil {
		pa.onDemandCmd.Close()
		pa.Log(logger.Info, "runOnDemand command stopped")
	}

	pa.Log(logger.Debug, "destroyed (%v)", err)
}

func (pa *path) runInner() error {
	for {
		select {
		case <-pa.onDemandStaticSourceReadyTimer.C:
			for _, req := range pa.describeRequestsOnHold {
				req.res <- pathDescribeRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
			}
			pa.describeRequestsOnHold = nil

			for _, req := range pa.readerAddRequestsOnHold {
				req.res <- pathAddReaderRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
			}
			pa.readerAddRequestsOnHold = nil

			pa.onDemandStaticSourceStop()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandStaticSourceCloseTimer.C:
			pa.setNotReady()
			pa.onDemandStaticSourceStop()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandPublisherReadyTimer.C:
			for _, req := range pa.describeRequestsOnHold {
				req.res <- pathDescribeRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
			}
			pa.describeRequestsOnHold = nil

			for _, req := range pa.readerAddRequestsOnHold {
				req.res <- pathAddReaderRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
			}
			pa.readerAddRequestsOnHold = nil

			pa.onDemandStopPublisher()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case <-pa.onDemandPublisherCloseTimer.C:
			pa.onDemandStopPublisher()

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case newConf := <-pa.chReloadConf:
			if pa.conf.HasStaticSource() {
				go pa.source.(*sourceStatic).reloadConf(newConf)
			}

			pa.confMutex.Lock()
			pa.conf = newConf
			pa.confMutex.Unlock()

		case req := <-pa.chSourceStaticSetReady:
			pa.handleSourceStaticSetReady(req)

		case req := <-pa.chSourceStaticSetNotReady:
			pa.handleSourceStaticSetNotReady(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chDescribe:
			pa.handleDescribe(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chRemovePublisher:
			pa.handleRemovePublisher(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chAddPublisher:
			pa.handleAddPublisher(req)

		case req := <-pa.chStartPublisher:
			pa.handleStartPublisher(req)

		case req := <-pa.chStopPublisher:
			pa.handleStopPublisher(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chAddReader:
			pa.handleAddReader(req)

			if pa.shouldClose() {
				return fmt.Errorf("not in use")
			}

		case req := <-pa.chRemoveReader:
			pa.handleRemoveReader(req)

		case req := <-pa.chAPIPathsGet:
			pa.handleAPIPathsGet(req)

		case <-pa.ctx.Done():
			return fmt.Errorf("terminated")
		}
	}
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
	pa.source.(*sourceStatic).start()

	pa.onDemandStaticSourceReadyTimer.Stop()
	pa.onDemandStaticSourceReadyTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandStartTimeout))

	pa.onDemandStaticSourceState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandStaticSourceScheduleClose() {
	pa.onDemandStaticSourceCloseTimer.Stop()
	pa.onDemandStaticSourceCloseTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandCloseAfter))

	pa.onDemandStaticSourceState = pathOnDemandStateClosing
}

func (pa *path) onDemandStaticSourceStop() {
	if pa.onDemandStaticSourceState == pathOnDemandStateClosing {
		pa.onDemandStaticSourceCloseTimer.Stop()
		pa.onDemandStaticSourceCloseTimer = newEmptyTimer()
	}

	pa.onDemandStaticSourceState = pathOnDemandStateInitial

	pa.source.(*sourceStatic).stop()
}

func (pa *path) onDemandStartPublisher() {
	pa.Log(logger.Info, "runOnDemand command started")
	pa.onDemandCmd = externalcmd.NewCmd(
		pa.externalCmdPool,
		pa.conf.RunOnDemand,
		pa.conf.RunOnDemandRestart,
		pa.externalCmdEnv(),
		func(err error) {
			pa.Log(logger.Info, "runOnDemand command exited: %v", err)
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

func (pa *path) onDemandStopPublisher() {
	if pa.source != nil {
		pa.source.(publisher).close()
		pa.doPublisherRemove()
	}

	if pa.onDemandPublisherState == pathOnDemandStateClosing {
		pa.onDemandPublisherCloseTimer.Stop()
		pa.onDemandPublisherCloseTimer = newEmptyTimer()
	}

	pa.onDemandPublisherState = pathOnDemandStateInitial

	if pa.onDemandCmd != nil {
		pa.onDemandCmd.Close()
		pa.onDemandCmd = nil
		pa.Log(logger.Info, "runOnDemand command stopped")
	}
}

func (pa *path) setReady(medias media.Medias, allocateEncoder bool) error {
	stream, err := stream.New(
		pa.udpMaxPayloadSize,
		medias,
		allocateEncoder,
		pa.bytesReceived,
		pa.source,
	)
	if err != nil {
		return err
	}

	pa.stream = stream
	pa.readyTime = time.Now()

	if pa.conf.RunOnReady != "" {
		pa.Log(logger.Info, "runOnReady command started")
		pa.onReadyCmd = externalcmd.NewCmd(
			pa.externalCmdPool,
			pa.conf.RunOnReady,
			pa.conf.RunOnReadyRestart,
			pa.externalCmdEnv(),
			func(err error) {
				pa.Log(logger.Info, "runOnReady command exited: %v", err)
			})
	}

	pa.parent.pathReady(pa)

	return nil
}

func (pa *path) setNotReady() {
	pa.parent.pathNotReady(pa)

	for r := range pa.readers {
		pa.doRemoveReader(r)
		r.close()
	}

	if pa.onReadyCmd != nil {
		pa.onReadyCmd.Close()
		pa.onReadyCmd = nil
		pa.Log(logger.Info, "runOnReady command stopped")
	}

	if pa.stream != nil {
		pa.stream.Close()
		pa.stream = nil
	}
}

func (pa *path) doRemoveReader(r reader) {
	delete(pa.readers, r)
}

func (pa *path) doPublisherRemove() {
	if pa.stream != nil {
		pa.setNotReady()
	}

	pa.source = nil
}

func (pa *path) handleSourceStaticSetReady(req pathSourceStaticSetReadyReq) {
	err := pa.setReady(req.medias, req.generateRTPPackets)
	if err != nil {
		req.res <- pathSourceStaticSetReadyRes{err: err}
		return
	}

	if pa.conf.HasOnDemandStaticSource() {
		pa.onDemandStaticSourceReadyTimer.Stop()
		pa.onDemandStaticSourceReadyTimer = newEmptyTimer()

		pa.onDemandStaticSourceScheduleClose()

		for _, req := range pa.describeRequestsOnHold {
			req.res <- pathDescribeRes{
				stream: pa.stream,
			}
		}
		pa.describeRequestsOnHold = nil

		for _, req := range pa.readerAddRequestsOnHold {
			pa.handleAddReaderPost(req)
		}
		pa.readerAddRequestsOnHold = nil
	}

	req.res <- pathSourceStaticSetReadyRes{stream: pa.stream}
}

func (pa *path) handleSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq) {
	pa.setNotReady()

	// send response before calling onDemandStaticSourceStop()
	// in order to avoid a deadlock due to sourceStatic.stop()
	close(req.res)

	if pa.conf.HasOnDemandStaticSource() && pa.onDemandStaticSourceState != pathOnDemandStateInitial {
		pa.onDemandStaticSourceStop()
	}
}

func (pa *path) handleDescribe(req pathDescribeReq) {
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
			pa.onDemandStartPublisher()
		}
		pa.describeRequestsOnHold = append(pa.describeRequestsOnHold, req)
		return
	}

	if pa.conf.Fallback != "" {
		fallbackURL := func() string {
			if strings.HasPrefix(pa.conf.Fallback, "/") {
				ur := url.URL{
					Scheme: req.url.Scheme,
					User:   req.url.User,
					Host:   req.url.Host,
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

func (pa *path) handleRemovePublisher(req pathRemovePublisherReq) {
	if pa.source == req.author {
		pa.doPublisherRemove()
	}
	close(req.res)
}

func (pa *path) handleAddPublisher(req pathAddPublisherReq) {
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
		pa.doPublisherRemove()
	}

	pa.source = req.author

	req.res <- pathAddPublisherRes{path: pa}
}

func (pa *path) handleStartPublisher(req pathStartPublisherReq) {
	if pa.source != req.author {
		req.res <- pathStartPublisherRes{err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	err := pa.setReady(req.medias, req.generateRTPPackets)
	if err != nil {
		req.res <- pathStartPublisherRes{err: err}
		return
	}

	req.author.Log(logger.Info, "is publishing to path '%s', %s",
		pa.name,
		sourceMediaInfo(req.medias))

	if pa.conf.HasOnDemandPublisher() {
		pa.onDemandPublisherReadyTimer.Stop()
		pa.onDemandPublisherReadyTimer = newEmptyTimer()

		pa.onDemandPublisherScheduleClose()

		for _, req := range pa.describeRequestsOnHold {
			req.res <- pathDescribeRes{
				stream: pa.stream,
			}
		}
		pa.describeRequestsOnHold = nil

		for _, req := range pa.readerAddRequestsOnHold {
			pa.handleAddReaderPost(req)
		}
		pa.readerAddRequestsOnHold = nil
	}

	req.res <- pathStartPublisherRes{stream: pa.stream}
}

func (pa *path) handleStopPublisher(req pathStopPublisherReq) {
	if req.author == pa.source && pa.stream != nil {
		pa.setNotReady()
	}
	close(req.res)
}

func (pa *path) handleRemoveReader(req pathRemoveReaderReq) {
	if _, ok := pa.readers[req.author]; ok {
		pa.doRemoveReader(req.author)
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

func (pa *path) handleAddReader(req pathAddReaderReq) {
	if pa.stream != nil {
		pa.handleAddReaderPost(req)
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
			pa.onDemandStartPublisher()
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	req.res <- pathAddReaderRes{err: errPathNoOnePublishing{pathName: pa.name}}
}

func (pa *path) handleAddReaderPost(req pathAddReaderReq) {
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

func (pa *path) handleAPIPathsGet(req pathAPIPathsGetReq) {
	req.res <- pathAPIPathsGetRes{
		data: &apiPath{
			Name:     pa.name,
			ConfName: pa.confName,
			Conf:     pa.conf,
			Source: func() interface{} {
				if pa.source == nil {
					return nil
				}
				return pa.source.apiSourceDescribe()
			}(),
			SourceReady: pa.stream != nil,
			Ready:       pa.stream != nil,
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
				return mediasDescription(pa.stream.Medias())
			}(),
			BytesReceived: atomic.LoadUint64(pa.bytesReceived),
			Readers: func() []interface{} {
				ret := []interface{}{}
				for r := range pa.readers {
					ret = append(ret, r.apiReaderDescribe())
				}
				return ret
			}(),
		},
	}
}

// reloadConf is called by pathManager.
func (pa *path) reloadConf(newConf *conf.PathConf) {
	select {
	case pa.chReloadConf <- newConf:
	case <-pa.ctx.Done():
	}
}

// sourceStaticSetReady is called by sourceStatic.
func (pa *path) sourceStaticSetReady(sourceStaticCtx context.Context, req pathSourceStaticSetReadyReq) {
	select {
	case pa.chSourceStaticSetReady <- req:

	case <-pa.ctx.Done():
		req.res <- pathSourceStaticSetReadyRes{err: fmt.Errorf("terminated")}

	// this avoids:
	// - invalid requests sent after the source has been terminated
	// - deadlocks caused by <-done inside stop()
	case <-sourceStaticCtx.Done():
		req.res <- pathSourceStaticSetReadyRes{err: fmt.Errorf("terminated")}
	}
}

// sourceStaticSetNotReady is called by sourceStatic.
func (pa *path) sourceStaticSetNotReady(sourceStaticCtx context.Context, req pathSourceStaticSetNotReadyReq) {
	select {
	case pa.chSourceStaticSetNotReady <- req:

	case <-pa.ctx.Done():
		close(req.res)

	// this avoids:
	// - invalid requests sent after the source has been terminated
	// - deadlocks caused by <-done inside stop()
	case <-sourceStaticCtx.Done():
		close(req.res)
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

// removePublisher is called by a publisher.
func (pa *path) removePublisher(req pathRemovePublisherReq) {
	req.res = make(chan struct{})
	select {
	case pa.chRemovePublisher <- req:
		<-req.res
	case <-pa.ctx.Done():
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
func (pa *path) apiPathsGet(req pathAPIPathsGetReq) (*apiPath, error) {
	req.res = make(chan pathAPIPathsGetRes)
	select {
	case pa.chAPIPathsGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-pa.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}
