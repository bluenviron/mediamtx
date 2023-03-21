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

	"github.com/aler9/gortsplib/v2/pkg/base"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/url"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func newEmptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

type authenticateFunc func(
	pathIPs []fmt.Stringer,
	pathUser conf.Credential,
	pathPass conf.Credential,
) error

type pathErrNoOnePublishing struct {
	pathName string
}

// Error implements the error interface.
func (e pathErrNoOnePublishing) Error() string {
	return fmt.Sprintf("no one is publishing to path '%s'", e.pathName)
}

type pathErrAuthNotCritical struct {
	message  string
	response *base.Response
}

// Error implements the error interface.
func (pathErrAuthNotCritical) Error() string {
	return "non-critical authentication error"
}

type pathErrAuthCritical struct {
	message  string
	response *base.Response
}

// Error implements the error interface.
func (pathErrAuthCritical) Error() string {
	return "critical authentication error"
}

type pathParent interface {
	log(logger.Level, string, ...interface{})
	pathSourceReady(*path)
	pathSourceNotReady(*path)
	onPathClose(*path)
}

type pathOnDemandState int

const (
	pathOnDemandStateInitial pathOnDemandState = iota
	pathOnDemandStateWaitingReady
	pathOnDemandStateReady
	pathOnDemandStateClosing
)

type pathSourceStaticSetReadyRes struct {
	stream *stream
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

type pathReaderRemoveReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherRemoveReq struct {
	author publisher
	res    chan struct{}
}

type pathDescribeRes struct {
	path     *path
	stream   *stream
	redirect string
	err      error
}

type pathDescribeReq struct {
	pathName     string
	url          *url.URL
	authenticate authenticateFunc
	res          chan pathDescribeRes
}

type pathReaderSetupPlayRes struct {
	path   *path
	stream *stream
	err    error
}

type pathReaderAddReq struct {
	author       reader
	pathName     string
	authenticate authenticateFunc
	res          chan pathReaderSetupPlayRes
}

type pathPublisherAnnounceRes struct {
	path *path
	err  error
}

type pathPublisherAddReq struct {
	author       publisher
	pathName     string
	authenticate authenticateFunc
	res          chan pathPublisherAnnounceRes
}

type pathPublisherRecordRes struct {
	stream *stream
	err    error
}

type pathPublisherStartReq struct {
	author             publisher
	medias             media.Medias
	generateRTPPackets bool
	res                chan pathPublisherRecordRes
}

type pathPublisherStopReq struct {
	author publisher
	res    chan struct{}
}

type pathAPIPathsListItem struct {
	ConfName      string         `json:"confName"`
	Conf          *conf.PathConf `json:"conf"`
	Source        interface{}    `json:"source"`
	SourceReady   bool           `json:"sourceReady"`
	Tracks        []string       `json:"tracks"`
	BytesReceived uint64         `json:"bytesReceived"`
	Readers       []interface{}  `json:"readers"`
}

type pathAPIPathsListData struct {
	Items map[string]pathAPIPathsListItem `json:"items"`
}

type pathAPIPathsListRes struct {
	data  *pathAPIPathsListData
	paths map[string]*path
	err   error
}

type pathAPIPathsListReq struct {
	res chan pathAPIPathsListRes
}

type pathAPIPathsListSubReq struct {
	data *pathAPIPathsListData
	res  chan struct{}
}

type path struct {
	rtspAddress     string
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	confName        string
	conf            *conf.PathConf
	name            string
	matches         []string
	wg              *sync.WaitGroup
	externalCmdPool *externalcmd.Pool
	parent          pathParent

	ctx                            context.Context
	ctxCancel                      func()
	confMutex                      sync.RWMutex
	source                         source
	bytesReceived                  *uint64
	stream                         *stream
	readers                        map[reader]struct{}
	describeRequestsOnHold         []pathDescribeReq
	readerAddRequestsOnHold        []pathReaderAddReq
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
	chPublisherRemove         chan pathPublisherRemoveReq
	chPublisherAdd            chan pathPublisherAddReq
	chPublisherStart          chan pathPublisherStartReq
	chPublisherStop           chan pathPublisherStopReq
	chReaderAdd               chan pathReaderAddReq
	chReaderRemove            chan pathReaderRemoveReq
	chAPIPathsList            chan pathAPIPathsListSubReq

	// out
	done chan struct{}
}

func newPath(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
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
		chPublisherRemove:              make(chan pathPublisherRemoveReq),
		chPublisherAdd:                 make(chan pathPublisherAddReq),
		chPublisherStart:               make(chan pathPublisherStartReq),
		chPublisherStop:                make(chan pathPublisherStopReq),
		chReaderAdd:                    make(chan pathReaderAddReq),
		chReaderRemove:                 make(chan pathReaderRemoveReq),
		chAPIPathsList:                 make(chan pathAPIPathsListSubReq),
		done:                           make(chan struct{}),
	}

	pa.log(logger.Debug, "created")

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
func (pa *path) log(level logger.Level, format string, args ...interface{}) {
	pa.parent.log(level, "[path "+pa.name+"] "+format, args...)
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
		pa.log(logger.Info, "runOnInit command started")
		onInitCmd = externalcmd.NewCmd(
			pa.externalCmdPool,
			pa.conf.RunOnInit,
			pa.conf.RunOnInitRestart,
			pa.externalCmdEnv(),
			func(co int) {
				pa.log(logger.Info, "runOnInit command exited with code %d", co)
			})
	}

	err := func() error {
		for {
			select {
			case <-pa.onDemandStaticSourceReadyTimer.C:
				for _, req := range pa.describeRequestsOnHold {
					req.res <- pathDescribeRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
				}
				pa.describeRequestsOnHold = nil

				for _, req := range pa.readerAddRequestsOnHold {
					req.res <- pathReaderSetupPlayRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
				}
				pa.readerAddRequestsOnHold = nil

				pa.onDemandStaticSourceStop()

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case <-pa.onDemandStaticSourceCloseTimer.C:
				pa.sourceSetNotReady()
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
					req.res <- pathReaderSetupPlayRes{err: fmt.Errorf("source of path '%s' has timed out", pa.name)}
				}
				pa.readerAddRequestsOnHold = nil

				pa.onDemandPublisherStop()

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case <-pa.onDemandPublisherCloseTimer.C:
				pa.onDemandPublisherStop()

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
				err := pa.sourceSetReady(req.medias, req.generateRTPPackets)
				if err != nil {
					req.res <- pathSourceStaticSetReadyRes{err: err}
				} else {
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
							pa.handleReaderAddPost(req)
						}
						pa.readerAddRequestsOnHold = nil
					}

					req.res <- pathSourceStaticSetReadyRes{stream: pa.stream}
				}

			case req := <-pa.chSourceStaticSetNotReady:
				pa.sourceSetNotReady()

				// send response before calling onDemandStaticSourceStop()
				// in order to avoid a deadlock due to sourceStatic.stop()
				close(req.res)

				if pa.conf.HasOnDemandStaticSource() && pa.onDemandStaticSourceState != pathOnDemandStateInitial {
					pa.onDemandStaticSourceStop()
				}

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.chDescribe:
				pa.handleDescribe(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.chPublisherRemove:
				pa.handlePublisherRemove(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.chPublisherAdd:
				pa.handlePublisherAdd(req)

			case req := <-pa.chPublisherStart:
				pa.handlePublisherStart(req)

			case req := <-pa.chPublisherStop:
				pa.handlePublisherStop(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.chReaderAdd:
				pa.handleReaderAdd(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.chReaderRemove:
				pa.handleReaderRemove(req)

			case req := <-pa.chAPIPathsList:
				pa.handleAPIPathsList(req)

			case <-pa.ctx.Done():
				return fmt.Errorf("terminated")
			}
		}
	}()

	// call before destroying context
	pa.parent.onPathClose(pa)

	pa.ctxCancel()

	pa.onDemandStaticSourceReadyTimer.Stop()
	pa.onDemandStaticSourceCloseTimer.Stop()
	pa.onDemandPublisherReadyTimer.Stop()
	pa.onDemandPublisherCloseTimer.Stop()

	if onInitCmd != nil {
		onInitCmd.Close()
		pa.log(logger.Info, "runOnInit command stopped")
	}

	for _, req := range pa.describeRequestsOnHold {
		req.res <- pathDescribeRes{err: fmt.Errorf("terminated")}
	}

	for _, req := range pa.readerAddRequestsOnHold {
		req.res <- pathReaderSetupPlayRes{err: fmt.Errorf("terminated")}
	}

	if pa.stream != nil {
		pa.sourceSetNotReady()
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
		pa.log(logger.Info, "runOnDemand command stopped")
	}

	pa.log(logger.Debug, "destroyed (%v)", err)
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
		"RTSP_PATH": pa.name,
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

func (pa *path) onDemandPublisherStart() {
	pa.log(logger.Info, "runOnDemand command started")
	pa.onDemandCmd = externalcmd.NewCmd(
		pa.externalCmdPool,
		pa.conf.RunOnDemand,
		pa.conf.RunOnDemandRestart,
		pa.externalCmdEnv(),
		func(co int) {
			pa.log(logger.Info, "runOnDemand command exited with code %d", co)
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

func (pa *path) onDemandPublisherStop() {
	if pa.onDemandPublisherState == pathOnDemandStateClosing {
		pa.onDemandPublisherCloseTimer.Stop()
		pa.onDemandPublisherCloseTimer = newEmptyTimer()
	}

	// set state before doPublisherRemove()
	pa.onDemandPublisherState = pathOnDemandStateInitial

	if pa.source != nil {
		pa.source.(publisher).close()
		pa.doPublisherRemove()
	}

	if pa.onDemandCmd != nil {
		pa.onDemandCmd.Close()
		pa.onDemandCmd = nil
		pa.log(logger.Info, "runOnDemand command stopped")
	}
}

func (pa *path) sourceSetReady(medias media.Medias, allocateEncoder bool) error {
	stream, err := newStream(medias, allocateEncoder, pa.bytesReceived)
	if err != nil {
		return err
	}

	pa.stream = stream

	if pa.conf.RunOnReady != "" {
		pa.log(logger.Info, "runOnReady command started")
		pa.onReadyCmd = externalcmd.NewCmd(
			pa.externalCmdPool,
			pa.conf.RunOnReady,
			pa.conf.RunOnReadyRestart,
			pa.externalCmdEnv(),
			func(co int) {
				pa.log(logger.Info, "runOnReady command exited with code %d", co)
			})
	}

	pa.parent.pathSourceReady(pa)

	return nil
}

func (pa *path) sourceSetNotReady() {
	pa.parent.pathSourceNotReady(pa)

	for r := range pa.readers {
		pa.doReaderRemove(r)
		r.close()
	}

	if pa.onReadyCmd != nil {
		pa.onReadyCmd.Close()
		pa.onReadyCmd = nil
		pa.log(logger.Info, "runOnReady command stopped")
	}

	if pa.stream != nil {
		pa.stream.close()
		pa.stream = nil
	}
}

func (pa *path) doReaderRemove(r reader) {
	delete(pa.readers, r)
}

func (pa *path) doPublisherRemove() {
	if pa.stream != nil {
		if pa.conf.HasOnDemandPublisher() && pa.onDemandPublisherState != pathOnDemandStateInitial {
			pa.onDemandPublisherStop()
		} else {
			pa.sourceSetNotReady()
		}
	}

	pa.source = nil
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
			pa.onDemandPublisherStart()
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

	req.res <- pathDescribeRes{err: pathErrNoOnePublishing{pathName: pa.name}}
}

func (pa *path) handlePublisherRemove(req pathPublisherRemoveReq) {
	if pa.source == req.author {
		pa.doPublisherRemove()
	}
	close(req.res)
}

func (pa *path) handlePublisherAdd(req pathPublisherAddReq) {
	if pa.conf.Source != "publisher" {
		req.res <- pathPublisherAnnounceRes{
			err: fmt.Errorf("can't publish to path '%s' since 'source' is not 'publisher'", pa.name),
		}
		return
	}

	if pa.source != nil {
		if pa.conf.DisablePublisherOverride {
			req.res <- pathPublisherAnnounceRes{err: fmt.Errorf("someone is already publishing to path '%s'", pa.name)}
			return
		}

		pa.log(logger.Info, "closing existing publisher")
		pa.source.(publisher).close()
		pa.doPublisherRemove()
	}

	pa.source = req.author

	req.res <- pathPublisherAnnounceRes{path: pa}
}

func (pa *path) handlePublisherStart(req pathPublisherStartReq) {
	if pa.source != req.author {
		req.res <- pathPublisherRecordRes{err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	err := pa.sourceSetReady(req.medias, req.generateRTPPackets)
	if err != nil {
		req.res <- pathPublisherRecordRes{err: err}
		return
	}

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
			pa.handleReaderAddPost(req)
		}
		pa.readerAddRequestsOnHold = nil
	}

	req.res <- pathPublisherRecordRes{stream: pa.stream}
}

func (pa *path) handlePublisherStop(req pathPublisherStopReq) {
	if req.author == pa.source && pa.stream != nil {
		if pa.conf.HasOnDemandPublisher() && pa.onDemandPublisherState != pathOnDemandStateInitial {
			pa.onDemandPublisherStop()
		} else {
			pa.sourceSetNotReady()
		}
	}
	close(req.res)
}

func (pa *path) handleReaderRemove(req pathReaderRemoveReq) {
	if _, ok := pa.readers[req.author]; ok {
		pa.doReaderRemove(req.author)
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

func (pa *path) handleReaderAdd(req pathReaderAddReq) {
	if pa.stream != nil {
		pa.handleReaderAddPost(req)
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
			pa.onDemandPublisherStart()
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	req.res <- pathReaderSetupPlayRes{err: pathErrNoOnePublishing{pathName: pa.name}}
}

func (pa *path) handleReaderAddPost(req pathReaderAddReq) {
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

	req.res <- pathReaderSetupPlayRes{
		path:   pa,
		stream: pa.stream,
	}
}

func (pa *path) handleAPIPathsList(req pathAPIPathsListSubReq) {
	req.data.Items[pa.name] = pathAPIPathsListItem{
		ConfName: pa.confName,
		Conf:     pa.conf,
		Source: func() interface{} {
			if pa.source == nil {
				return nil
			}
			return pa.source.apiSourceDescribe()
		}(),
		SourceReady: pa.stream != nil,
		Tracks: func() []string {
			if pa.stream == nil {
				return []string{}
			}
			return mediasDescription(pa.stream.medias())
		}(),
		BytesReceived: atomic.LoadUint64(pa.bytesReceived),
		Readers: func() []interface{} {
			ret := []interface{}{}
			for r := range pa.readers {
				ret = append(ret, r.apiReaderDescribe())
			}
			return ret
		}(),
	}
	close(req.res)
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

// publisherRemove is called by a publisher.
func (pa *path) publisherRemove(req pathPublisherRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.chPublisherRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// publisherAdd is called by a publisher through pathManager.
func (pa *path) publisherAdd(req pathPublisherAddReq) pathPublisherAnnounceRes {
	select {
	case pa.chPublisherAdd <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherAnnounceRes{err: fmt.Errorf("terminated")}
	}
}

// publisherStart is called by a publisher.
func (pa *path) publisherStart(req pathPublisherStartReq) pathPublisherRecordRes {
	req.res = make(chan pathPublisherRecordRes)
	select {
	case pa.chPublisherStart <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherRecordRes{err: fmt.Errorf("terminated")}
	}
}

// publisherStop is called by a publisher.
func (pa *path) publisherStop(req pathPublisherStopReq) {
	req.res = make(chan struct{})
	select {
	case pa.chPublisherStop <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// readerAdd is called by a reader through pathManager.
func (pa *path) readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes {
	select {
	case pa.chReaderAdd <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathReaderSetupPlayRes{err: fmt.Errorf("terminated")}
	}
}

// readerRemove is called by a reader.
func (pa *path) readerRemove(req pathReaderRemoveReq) {
	req.res = make(chan struct{})
	select {
	case pa.chReaderRemove <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// apiPathsList is called by api.
func (pa *path) apiPathsList(req pathAPIPathsListSubReq) {
	req.res = make(chan struct{})
	select {
	case pa.chAPIPathsList <- req:
		<-req.res

	case <-pa.ctx.Done():
	}
}
