package core

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/url"

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

type pathRTSPSession interface {
	isRTSPSession()
}

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

type pathSourceStaticSetReadyRes struct {
	stream *stream
	err    error
}

type pathSourceStaticSetReadyReq struct {
	tracks             gortsplib.Tracks
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

type pathReaderStartReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherRecordRes struct {
	stream *stream
	err    error
}

type pathPublisherStartReq struct {
	author             publisher
	tracks             gortsplib.Tracks
	generateRTPPackets bool
	res                chan pathPublisherRecordRes
}

type pathReaderStopReq struct {
	author reader
	res    chan struct{}
}

type pathPublisherStopReq struct {
	author publisher
	res    chan struct{}
}

type pathAPIPathsListItem struct {
	ConfName    string         `json:"confName"`
	Conf        *conf.PathConf `json:"conf"`
	Source      interface{}    `json:"source"`
	SourceReady bool           `json:"sourceReady"`
	Tracks      []string       `json:"tracks"`
	Readers     []interface{}  `json:"readers"`
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
	source                         source
	stream                         *stream
	readers                        map[reader]pathReaderState
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
	chSourceStaticSetReady    chan pathSourceStaticSetReadyReq
	chSourceStaticSetNotReady chan pathSourceStaticSetNotReadyReq
	chDescribe                chan pathDescribeReq
	chPublisherRemove         chan pathPublisherRemoveReq
	chPublisherAdd            chan pathPublisherAddReq
	chPublisherStart          chan pathPublisherStartReq
	chPublisherStop           chan pathPublisherStopReq
	chReaderRemove            chan pathReaderRemoveReq
	chReaderAdd               chan pathReaderAddReq
	chReaderStart             chan pathReaderStartReq
	chReaderStop              chan pathReaderStopReq
	chAPIPathsList            chan pathAPIPathsListSubReq
}

func newPath(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	confName string,
	conf *conf.PathConf,
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
		conf:                           conf,
		name:                           name,
		matches:                        matches,
		wg:                             wg,
		externalCmdPool:                externalCmdPool,
		parent:                         parent,
		ctx:                            ctx,
		ctxCancel:                      ctxCancel,
		readers:                        make(map[reader]pathReaderState),
		onDemandStaticSourceReadyTimer: newEmptyTimer(),
		onDemandStaticSourceCloseTimer: newEmptyTimer(),
		onDemandPublisherReadyTimer:    newEmptyTimer(),
		onDemandPublisherCloseTimer:    newEmptyTimer(),
		chSourceStaticSetReady:         make(chan pathSourceStaticSetReadyReq),
		chSourceStaticSetNotReady:      make(chan pathSourceStaticSetNotReadyReq),
		chDescribe:                     make(chan pathDescribeReq),
		chPublisherRemove:              make(chan pathPublisherRemoveReq),
		chPublisherAdd:                 make(chan pathPublisherAddReq),
		chPublisherStart:               make(chan pathPublisherStartReq),
		chPublisherStop:                make(chan pathPublisherStopReq),
		chReaderRemove:                 make(chan pathReaderRemoveReq),
		chReaderAdd:                    make(chan pathReaderAddReq),
		chReaderStart:                  make(chan pathReaderStartReq),
		chReaderStop:                   make(chan pathReaderStopReq),
		chAPIPathsList:                 make(chan pathAPIPathsListSubReq),
	}

	pa.log(logger.Debug, "created")

	pa.wg.Add(1)
	go pa.run()

	return pa
}

func (pa *path) close() {
	pa.ctxCancel()
}

// Log is the main logging function.
func (pa *path) log(level logger.Level, format string, args ...interface{}) {
	pa.parent.log(level, "[path "+pa.name+"] "+format, args...)
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

func (pa *path) hasStaticSource() bool {
	return strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") ||
		strings.HasPrefix(pa.conf.Source, "rtmp://") ||
		strings.HasPrefix(pa.conf.Source, "rtmps://") ||
		strings.HasPrefix(pa.conf.Source, "http://") ||
		strings.HasPrefix(pa.conf.Source, "https://") ||
		pa.conf.Source == "rpiCamera"
}

func (pa *path) hasOnDemandStaticSource() bool {
	return pa.hasStaticSource() && pa.conf.SourceOnDemand
}

func (pa *path) hasOnDemandPublisher() bool {
	return pa.conf.RunOnDemand != ""
}

func (pa *path) run() {
	defer pa.wg.Done()

	if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	} else if pa.hasStaticSource() {
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

			case req := <-pa.chSourceStaticSetReady:
				err := pa.sourceSetReady(req.tracks, req.generateRTPPackets)
				if err != nil {
					req.res <- pathSourceStaticSetReadyRes{err: err}
				} else {
					if pa.hasOnDemandStaticSource() {
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
							pa.handleReaderSetupPlayPost(req)
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

				if pa.hasOnDemandStaticSource() && pa.onDemandStaticSourceState != pathOnDemandStateInitial {
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
				pa.handlePublisherAnnounce(req)

			case req := <-pa.chPublisherStart:
				pa.handlePublisherRecord(req)

			case req := <-pa.chPublisherStop:
				pa.handlePublisherPause(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.chReaderRemove:
				pa.handleReaderRemove(req)

			case req := <-pa.chReaderAdd:
				pa.handleReaderSetupPlay(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.chReaderStart:
				pa.handleReaderPlay(req)

			case req := <-pa.chReaderStop:
				pa.handleReaderPause(req)

			case req := <-pa.chAPIPathsList:
				pa.handleAPIPathsList(req)

			case <-pa.ctx.Done():
				return fmt.Errorf("terminated")
			}
		}
	}()

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

	pa.parent.onPathClose(pa)
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

func (pa *path) sourceSetReady(tracks gortsplib.Tracks, generateRTPPackets bool) error {
	stream, err := newStream(tracks, generateRTPPackets)
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
	state := pa.readers[r]

	if state == pathReaderStatePlay {
		pa.stream.readerRemove(r)
	}

	delete(pa.readers, r)
}

func (pa *path) doPublisherRemove() {
	if pa.stream != nil {
		if pa.hasOnDemandPublisher() && pa.onDemandPublisherState != pathOnDemandStateInitial {
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

	if pa.hasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateInitial {
			pa.onDemandStaticSourceStart()
		}
		pa.describeRequestsOnHold = append(pa.describeRequestsOnHold, req)
		return
	}

	if pa.hasOnDemandPublisher() {
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

func (pa *path) handlePublisherAnnounce(req pathPublisherAddReq) {
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

func (pa *path) handlePublisherRecord(req pathPublisherStartReq) {
	if pa.source != req.author {
		req.res <- pathPublisherRecordRes{err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	err := pa.sourceSetReady(req.tracks, req.generateRTPPackets)
	if err != nil {
		req.res <- pathPublisherRecordRes{err: err}
		return
	}

	if pa.hasOnDemandPublisher() {
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
			pa.handleReaderSetupPlayPost(req)
		}
		pa.readerAddRequestsOnHold = nil
	}

	req.res <- pathPublisherRecordRes{stream: pa.stream}
}

func (pa *path) handlePublisherPause(req pathPublisherStopReq) {
	if req.author == pa.source && pa.stream != nil {
		if pa.hasOnDemandPublisher() && pa.onDemandPublisherState != pathOnDemandStateInitial {
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
		if pa.hasOnDemandStaticSource() {
			if pa.onDemandStaticSourceState == pathOnDemandStateReady {
				pa.onDemandStaticSourceScheduleClose()
			}
		} else if pa.hasOnDemandPublisher() {
			if pa.onDemandPublisherState == pathOnDemandStateReady {
				pa.onDemandPublisherScheduleClose()
			}
		}
	}
}

func (pa *path) handleReaderSetupPlay(req pathReaderAddReq) {
	if pa.stream != nil {
		pa.handleReaderSetupPlayPost(req)
		return
	}

	if pa.hasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateInitial {
			pa.onDemandStaticSourceStart()
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	if pa.hasOnDemandPublisher() {
		if pa.onDemandPublisherState == pathOnDemandStateInitial {
			pa.onDemandPublisherStart()
		}
		pa.readerAddRequestsOnHold = append(pa.readerAddRequestsOnHold, req)
		return
	}

	req.res <- pathReaderSetupPlayRes{err: pathErrNoOnePublishing{pathName: pa.name}}
}

func (pa *path) handleReaderSetupPlayPost(req pathReaderAddReq) {
	pa.readers[req.author] = pathReaderStatePrePlay

	if pa.hasOnDemandStaticSource() {
		if pa.onDemandStaticSourceState == pathOnDemandStateClosing {
			pa.onDemandStaticSourceState = pathOnDemandStateReady
			pa.onDemandStaticSourceCloseTimer.Stop()
			pa.onDemandStaticSourceCloseTimer = newEmptyTimer()
		}
	} else if pa.hasOnDemandPublisher() {
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

func (pa *path) handleReaderPlay(req pathReaderStartReq) {
	pa.readers[req.author] = pathReaderStatePlay

	pa.stream.readerAdd(req.author)

	close(req.res)
}

func (pa *path) handleReaderPause(req pathReaderStopReq) {
	if state, ok := pa.readers[req.author]; ok && state == pathReaderStatePlay {
		pa.readers[req.author] = pathReaderStatePrePlay
		pa.stream.readerRemove(req.author)
	}
	close(req.res)
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
			return sourceTrackNames(pa.stream.tracks())
		}(),
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

// publisherAnnounce is called by a publisher through pathManager.
func (pa *path) publisherAdd(req pathPublisherAddReq) pathPublisherAnnounceRes {
	select {
	case pa.chPublisherAdd <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherAnnounceRes{err: fmt.Errorf("terminated")}
	}
}

// publisherRecord is called by a publisher.
func (pa *path) publisherStart(req pathPublisherStartReq) pathPublisherRecordRes {
	req.res = make(chan pathPublisherRecordRes)
	select {
	case pa.chPublisherStart <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathPublisherRecordRes{err: fmt.Errorf("terminated")}
	}
}

// publisherPause is called by a publisher.
func (pa *path) publisherStop(req pathPublisherStopReq) {
	req.res = make(chan struct{})
	select {
	case pa.chPublisherStop <- req:
		<-req.res
	case <-pa.ctx.Done():
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

// readerSetupPlay is called by a reader through pathManager.
func (pa *path) readerAdd(req pathReaderAddReq) pathReaderSetupPlayRes {
	select {
	case pa.chReaderAdd <- req:
		return <-req.res
	case <-pa.ctx.Done():
		return pathReaderSetupPlayRes{err: fmt.Errorf("terminated")}
	}
}

// readerPlay is called by a reader.
func (pa *path) readerStart(req pathReaderStartReq) {
	req.res = make(chan struct{})
	select {
	case pa.chReaderStart <- req:
		<-req.res
	case <-pa.ctx.Done():
	}
}

// readerPause is called by a reader.
func (pa *path) readerStop(req pathReaderStopReq) {
	req.res = make(chan struct{})
	select {
	case pa.chReaderStop <- req:
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
