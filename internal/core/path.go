package core

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
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
	log(logger.Level, string, ...interface{})
	onPathSourceReady(*path)
	onPathClose(*path)
}

type pathRTSPSession interface {
	IsRTSPSession()
}

type sourceRedirect struct{}

// onSourceAPIDescribe implements source.
func (*sourceRedirect) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"redirect"}
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
	Stream *stream
	Err    error
}

type pathSourceStaticSetReadyReq struct {
	Source sourceStatic
	Tracks gortsplib.Tracks
	Res    chan pathSourceStaticSetReadyRes
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
	Stream   *stream
	Redirect string
	Err      error
}

type pathDescribeReq struct {
	PathName            string
	URL                 *base.URL
	IP                  net.IP
	ValidateCredentials func(pathUser conf.Credential, pathPass conf.Credential) error
	Res                 chan pathDescribeRes
}

type pathReaderSetupPlayRes struct {
	Path   *path
	Stream *stream
	Err    error
}

type pathReaderSetupPlayReq struct {
	Author              reader
	PathName            string
	IP                  net.IP
	ValidateCredentials func(pathUser conf.Credential, pathPass conf.Credential) error
	Res                 chan pathReaderSetupPlayRes
}

type pathPublisherAnnounceRes struct {
	Path *path
	Err  error
}

type pathPublisherAnnounceReq struct {
	Author              publisher
	PathName            string
	IP                  net.IP
	ValidateCredentials func(pathUser conf.Credential, pathPass conf.Credential) error
	Res                 chan pathPublisherAnnounceRes
}

type pathReaderPlayReq struct {
	Author reader
	Res    chan struct{}
}

type pathPublisherRecordRes struct {
	Stream *stream
	Err    error
}

type pathPublisherRecordReq struct {
	Author publisher
	Tracks gortsplib.Tracks
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

type pathAPIPathsListItem struct {
	ConfName    string         `json:"confName"`
	Conf        *conf.PathConf `json:"conf"`
	Source      interface{}    `json:"source"`
	SourceReady bool           `json:"sourceReady"`
	Readers     []interface{}  `json:"readers"`
}

type pathAPIPathsListData struct {
	Items map[string]pathAPIPathsListItem `json:"items"`
}

type pathAPIPathsListRes struct {
	Data  *pathAPIPathsListData
	Paths map[string]*path
	Err   error
}

type pathAPIPathsListReq struct {
	Res chan pathAPIPathsListRes
}

type pathAPIPathsListSubReq struct {
	Data *pathAPIPathsListData
	Res  chan struct{}
}

type path struct {
	rtspAddress     string
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	readBufferSize  int
	confName        string
	conf            *conf.PathConf
	name            string
	wg              *sync.WaitGroup
	parent          pathParent

	ctx                context.Context
	ctxCancel          func()
	source             source
	sourceReady        bool
	sourceStaticWg     sync.WaitGroup
	readers            map[reader]pathReaderState
	describeRequests   []pathDescribeReq
	setupPlayRequests  []pathReaderSetupPlayReq
	stream             *stream
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
	apiPathsList            chan pathAPIPathsListSubReq
}

func newPath(
	parentCtx context.Context,
	rtspAddress string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	readBufferSize int,
	confName string,
	conf *conf.PathConf,
	name string,
	wg *sync.WaitGroup,
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
		parent:                  parent,
		ctx:                     ctx,
		ctxCancel:               ctxCancel,
		readers:                 make(map[reader]pathReaderState),
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
		apiPathsList:            make(chan pathAPIPathsListSubReq),
	}

	pa.log(logger.Debug, "opened")

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

func (pa *path) run() {
	defer pa.wg.Done()

	if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	} else if !pa.conf.SourceOnDemand && pa.hasStaticSource() {
		pa.staticSourceCreate()
	}

	var onInitCmd *externalcmd.Cmd
	if pa.conf.RunOnInit != "" {
		pa.log(logger.Info, "runOnInit command started")
		_, port, _ := net.SplitHostPort(pa.rtspAddress)
		onInitCmd = externalcmd.New(pa.conf.RunOnInit, pa.conf.RunOnInitRestart, externalcmd.Environment{
			Path: pa.name,
			Port: port,
		})
	}

	err := func() error {
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

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case <-pa.onDemandCloseTimer.C:
				pa.onDemandCloseSource()

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.sourceStaticSetReady:
				if req.Source == pa.source {
					pa.sourceSetReady(req.Tracks)
					req.Res <- pathSourceStaticSetReadyRes{Stream: pa.stream}
				} else {
					req.Res <- pathSourceStaticSetReadyRes{Err: fmt.Errorf("terminated")}
				}

			case req := <-pa.sourceStaticSetNotReady:
				if req.Source == pa.source {
					if pa.isOnDemand() && pa.onDemandState != pathOnDemandStateInitial {
						pa.onDemandCloseSource()
					} else {
						pa.sourceSetNotReady()
					}
				}
				close(req.Res)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.describe:
				pa.handleDescribe(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.publisherRemove:
				pa.handlePublisherRemove(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.publisherAnnounce:
				pa.handlePublisherAnnounce(req)

			case req := <-pa.publisherRecord:
				pa.handlePublisherRecord(req)

			case req := <-pa.publisherPause:
				pa.handlePublisherPause(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.readerRemove:
				pa.handleReaderRemove(req)

			case req := <-pa.readerSetupPlay:
				pa.handleReaderSetupPlay(req)

				if pa.shouldClose() {
					return fmt.Errorf("not in use")
				}

			case req := <-pa.readerPlay:
				pa.handleReaderPlay(req)

			case req := <-pa.readerPause:
				pa.handleReaderPause(req)

			case req := <-pa.apiPathsList:
				pa.handleAPIPathsList(req)

			case <-pa.ctx.Done():
				return fmt.Errorf("terminated")
			}
		}
	}()

	pa.ctxCancel()

	pa.onDemandReadyTimer.Stop()
	pa.onDemandCloseTimer.Stop()

	if onInitCmd != nil {
		onInitCmd.Close()
		pa.log(logger.Info, "runOnInit command stopped")
	}

	for _, req := range pa.describeRequests {
		req.Res <- pathDescribeRes{Err: fmt.Errorf("terminated")}
	}

	for _, req := range pa.setupPlayRequests {
		req.Res <- pathReaderSetupPlayRes{Err: fmt.Errorf("terminated")}
	}

	for rp := range pa.readers {
		rp.close()
	}

	if pa.stream != nil {
		pa.stream.close()
	}

	if pa.source != nil {
		if source, ok := pa.source.(sourceStatic); ok {
			source.close()
			pa.sourceStaticWg.Wait()
		} else if source, ok := pa.source.(publisher); ok {
			source.close()
		}
	}

	// close onDemandCmd after the source has been closed.
	// this avoids a deadlock in which onDemandCmd is a
	// RTSP publisher that sends a TEARDOWN request and waits
	// for the response (like FFmpeg), but it can't since
	// the path is already waiting for the command to close.
	if pa.onDemandCmd != nil {
		pa.onDemandCmd.Close()
		pa.log(logger.Info, "runOnDemand command stopped")
	}

	pa.log(logger.Debug, "closed (%v)", err)

	pa.parent.onPathClose(pa)
}

func (pa *path) shouldClose() bool {
	return pa.conf.Regexp != nil &&
		pa.source == nil &&
		len(pa.readers) == 0 &&
		len(pa.describeRequests) == 0 &&
		len(pa.setupPlayRequests) == 0
}

func (pa *path) hasStaticSource() bool {
	return strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") ||
		strings.HasPrefix(pa.conf.Source, "rtmp://") ||
		strings.HasPrefix(pa.conf.Source, "http://") ||
		strings.HasPrefix(pa.conf.Source, "https://")
}

func (pa *path) isOnDemand() bool {
	return (pa.hasStaticSource() && pa.conf.SourceOnDemand) || pa.conf.RunOnDemand != ""
}

func (pa *path) onDemandStartSource() {
	pa.onDemandReadyTimer.Stop()
	if pa.hasStaticSource() {
		pa.staticSourceCreate()
		pa.onDemandReadyTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandStartTimeout))
	} else {
		pa.log(logger.Info, "runOnDemand command started")
		_, port, _ := net.SplitHostPort(pa.rtspAddress)
		pa.onDemandCmd = externalcmd.New(pa.conf.RunOnDemand, pa.conf.RunOnDemandRestart, externalcmd.Environment{
			Path: pa.name,
			Port: port,
		})
		pa.onDemandReadyTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandStartTimeout))
	}

	pa.onDemandState = pathOnDemandStateWaitingReady
}

func (pa *path) onDemandScheduleClose() {
	pa.onDemandCloseTimer.Stop()
	if pa.hasStaticSource() {
		pa.onDemandCloseTimer = time.NewTimer(time.Duration(pa.conf.SourceOnDemandCloseAfter))
	} else {
		pa.onDemandCloseTimer = time.NewTimer(time.Duration(pa.conf.RunOnDemandCloseAfter))
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
		if pa.sourceReady {
			pa.sourceSetNotReady()
		}
		pa.source.(sourceStatic).close()
		pa.source = nil
	} else {
		if pa.source != nil {
			pa.source.(publisher).close()
			pa.doPublisherRemove()
		}

		// close onDemandCmd after the source has been closed.
		// this avoids a deadlock in which onDemandCmd is a
		// RTSP publisher that sends a TEARDOWN request and waits
		// for the response (like FFmpeg), but it can't since
		// the path is already waiting for the command to close.
		if pa.onDemandCmd != nil {
			pa.onDemandCmd.Close()
			pa.onDemandCmd = nil
			pa.log(logger.Info, "runOnDemand command stopped")
		}
	}
}

func (pa *path) sourceSetReady(tracks gortsplib.Tracks) {
	pa.sourceReady = true
	pa.stream = newStream(tracks)

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
			pa.handleReaderSetupPlayPost(req)
		}
		pa.setupPlayRequests = nil

		if len(pa.readers) > 0 {
			pa.onDemandState = pathOnDemandStateReady
		} else {
			pa.onDemandScheduleClose()
		}
	}

	pa.parent.onPathSourceReady(pa)
}

func (pa *path) sourceSetNotReady() {
	for r := range pa.readers {
		pa.doReaderRemove(r)
		r.close()
	}

	// close onPublishCmd after all readers have been closed.
	// this avoids a deadlock in which onPublishCmd is a
	// RTSP reader that sends a TEARDOWN request and waits
	// for the response (like FFmpeg), but it can't since
	// the path is already waiting for the command to close.
	if pa.onPublishCmd != nil {
		pa.onPublishCmd.Close()
		pa.onPublishCmd = nil
		pa.log(logger.Info, "runOnPublish command stopped")
	}

	pa.sourceReady = false
	pa.stream.close()
	pa.stream = nil
}

func (pa *path) staticSourceCreate() {
	switch {
	case strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://"):
		pa.source = newRTSPSource(
			pa.ctx,
			pa.conf.Source,
			pa.conf.SourceProtocol,
			pa.conf.SourceAnyPortEnable,
			pa.conf.SourceFingerprint,
			pa.readTimeout,
			pa.writeTimeout,
			pa.readBufferCount,
			pa.readBufferSize,
			&pa.sourceStaticWg,
			pa)
	case strings.HasPrefix(pa.conf.Source, "rtmp://"):
		pa.source = newRTMPSource(
			pa.ctx,
			pa.conf.Source,
			pa.readTimeout,
			pa.writeTimeout,
			&pa.sourceStaticWg,
			pa)
	case strings.HasPrefix(pa.conf.Source, "http://") ||
		strings.HasPrefix(pa.conf.Source, "https://"):
		pa.source = newHLSSource(
			pa.ctx,
			pa.conf.Source,
			pa.conf.SourceFingerprint,
			&pa.sourceStaticWg,
			pa)
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
	if pa.sourceReady {
		if pa.isOnDemand() && pa.onDemandState != pathOnDemandStateInitial {
			pa.onDemandCloseSource()
		} else {
			pa.sourceSetNotReady()
		}
	} else {
		for r := range pa.readers {
			pa.doReaderRemove(r)
			r.close()
		}
	}

	pa.source = nil
}

func (pa *path) handleDescribe(req pathDescribeReq) {
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

func (pa *path) handlePublisherRemove(req pathPublisherRemoveReq) {
	if pa.source == req.Author {
		pa.doPublisherRemove()
	}
	close(req.Res)
}

func (pa *path) handlePublisherAnnounce(req pathPublisherAnnounceReq) {
	if pa.source != nil {
		if pa.hasStaticSource() {
			req.Res <- pathPublisherAnnounceRes{Err: fmt.Errorf("path '%s' is assigned to a static source", pa.name)}
			return
		}

		if pa.conf.DisablePublisherOverride {
			req.Res <- pathPublisherAnnounceRes{Err: fmt.Errorf("another publisher is already publishing to path '%s'", pa.name)}
			return
		}

		pa.log(logger.Info, "closing existing publisher")
		pa.source.(publisher).close()
		pa.doPublisherRemove()
	}

	pa.source = req.Author

	req.Res <- pathPublisherAnnounceRes{Path: pa}
}

func (pa *path) handlePublisherRecord(req pathPublisherRecordReq) {
	if pa.source != req.Author {
		req.Res <- pathPublisherRecordRes{Err: fmt.Errorf("publisher is not assigned to this path anymore")}
		return
	}

	req.Author.onPublisherAccepted(len(req.Tracks))

	pa.sourceSetReady(req.Tracks)

	if pa.conf.RunOnPublish != "" {
		pa.log(logger.Info, "runOnPublish command started")
		_, port, _ := net.SplitHostPort(pa.rtspAddress)
		pa.onPublishCmd = externalcmd.New(pa.conf.RunOnPublish, pa.conf.RunOnPublishRestart, externalcmd.Environment{
			Path: pa.name,
			Port: port,
		})
	}

	req.Res <- pathPublisherRecordRes{Stream: pa.stream}
}

func (pa *path) handlePublisherPause(req pathPublisherPauseReq) {
	if req.Author == pa.source && pa.sourceReady {
		if pa.isOnDemand() && pa.onDemandState != pathOnDemandStateInitial {
			pa.onDemandCloseSource()
		} else {
			pa.sourceSetNotReady()
		}
	}
	close(req.Res)
}

func (pa *path) handleReaderRemove(req pathReaderRemoveReq) {
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

func (pa *path) handleReaderSetupPlay(req pathReaderSetupPlayReq) {
	if pa.sourceReady {
		pa.handleReaderSetupPlayPost(req)
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

func (pa *path) handleReaderSetupPlayPost(req pathReaderSetupPlayReq) {
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

func (pa *path) handleReaderPlay(req pathReaderPlayReq) {
	pa.readers[req.Author] = pathReaderStatePlay

	pa.stream.readerAdd(req.Author)

	req.Author.onReaderAccepted()

	close(req.Res)
}

func (pa *path) handleReaderPause(req pathReaderPauseReq) {
	if state, ok := pa.readers[req.Author]; ok && state == pathReaderStatePlay {
		pa.readers[req.Author] = pathReaderStatePrePlay
		pa.stream.readerRemove(req.Author)
	}
	close(req.Res)
}

func (pa *path) handleAPIPathsList(req pathAPIPathsListSubReq) {
	req.Data.Items[pa.name] = pathAPIPathsListItem{
		ConfName: pa.confName,
		Conf:     pa.conf,
		Source: func() interface{} {
			if pa.source == nil {
				return nil
			}
			return pa.source.onSourceAPIDescribe()
		}(),
		SourceReady: pa.sourceReady,
		Readers: func() []interface{} {
			ret := []interface{}{}
			for r := range pa.readers {
				ret = append(ret, r.onReaderAPIDescribe())
			}
			return ret
		}(),
	}
	close(req.Res)
}

// onSourceStaticSetReady is called by a sourceStatic.
func (pa *path) onSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes {
	req.Res = make(chan pathSourceStaticSetReadyRes)
	select {
	case pa.sourceStaticSetReady <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathSourceStaticSetReadyRes{Err: fmt.Errorf("terminated")}
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

// onDescribe is called by a reader or publisher through pathManager.
func (pa *path) onDescribe(req pathDescribeReq) pathDescribeRes {
	select {
	case pa.describe <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathDescribeRes{Err: fmt.Errorf("terminated")}
	}
}

// onPublisherRemove is called by a publisher.
func (pa *path) onPublisherRemove(req pathPublisherRemoveReq) {
	req.Res = make(chan struct{})
	select {
	case pa.publisherRemove <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onPublisherAnnounce is called by a publisher through pathManager.
func (pa *path) onPublisherAnnounce(req pathPublisherAnnounceReq) pathPublisherAnnounceRes {
	select {
	case pa.publisherAnnounce <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathPublisherAnnounceRes{Err: fmt.Errorf("terminated")}
	}
}

// onPublisherRecord is called by a publisher.
func (pa *path) onPublisherRecord(req pathPublisherRecordReq) pathPublisherRecordRes {
	req.Res = make(chan pathPublisherRecordRes)
	select {
	case pa.publisherRecord <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathPublisherRecordRes{Err: fmt.Errorf("terminated")}
	}
}

// onPublisherPause is called by a publisher.
func (pa *path) onPublisherPause(req pathPublisherPauseReq) {
	req.Res = make(chan struct{})
	select {
	case pa.publisherPause <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onReaderRemove is called by a reader.
func (pa *path) onReaderRemove(req pathReaderRemoveReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerRemove <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onReaderSetupPlay is called by a reader through pathManager.
func (pa *path) onReaderSetupPlay(req pathReaderSetupPlayReq) pathReaderSetupPlayRes {
	select {
	case pa.readerSetupPlay <- req:
		return <-req.Res
	case <-pa.ctx.Done():
		return pathReaderSetupPlayRes{Err: fmt.Errorf("terminated")}
	}
}

// onReaderPlay is called by a reader.
func (pa *path) onReaderPlay(req pathReaderPlayReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerPlay <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onReaderPause is called by a reader.
func (pa *path) onReaderPause(req pathReaderPauseReq) {
	req.Res = make(chan struct{})
	select {
	case pa.readerPause <- req:
		<-req.Res
	case <-pa.ctx.Done():
	}
}

// onAPIPathsList is called by api.
func (pa *path) onAPIPathsList(req pathAPIPathsListSubReq) {
	req.Res = make(chan struct{})
	select {
	case pa.apiPathsList <- req:
		<-req.Res

	case <-pa.ctx.Done():
	}
}
