package path

import (
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
	"github.com/aler9/rtsp-simple-server/internal/readpublisher"
	"github.com/aler9/rtsp-simple-server/internal/source"
	"github.com/aler9/rtsp-simple-server/internal/rtmpsource"
	"github.com/aler9/rtsp-simple-server/internal/rtspsource"
	"github.com/aler9/rtsp-simple-server/internal/stats"
	"github.com/aler9/rtsp-simple-server/internal/streamproc"
)

func newEmptyTimer() *time.Timer {
	t := time.NewTimer(0)
	<-t.C
	return t
}

// Parent is implemented by pathman.PathMan.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnPathClose(*Path)
}

type sourceRedirect struct{}

func (*sourceRedirect) IsSource() {}

type readPublisherState int

const (
	readPublisherStatePrePlay readPublisherState = iota
	readPublisherStatePlay
	readPublisherStatePreRecord
	readPublisherStateRecord
	readPublisherStatePreRemove
)

type sourceState int

const (
	sourceStateNotReady sourceState = iota
	sourceStateWaitingDescribe
	sourceStateReady
)

// Path is a path.
type Path struct {
	rtspAddress     string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	readBufferSize  int
	confName        string
	conf            *conf.PathConf
	name            string
	wg              *sync.WaitGroup
	stats           *stats.Stats
	parent          Parent

	readPublishers               map[readpublisher.ReadPublisher]readPublisherState
	readPublishersWg             sync.WaitGroup
	describeRequests             []readpublisher.DescribeReq
	setupPlayRequests            []readpublisher.SetupPlayReq
	source                       source.Source
	sourceTracks                 gortsplib.Tracks
	sp                           *streamproc.StreamProc
	readers                      *readersMap
	onDemandCmd                  *externalcmd.Cmd
	describeTimer                *time.Timer
	sourceCloseTimer             *time.Timer
	sourceCloseTimerStarted      bool
	sourceState                  sourceState
	sourceWg                     sync.WaitGroup
	runOnDemandCloseTimer        *time.Timer
	runOnDemandCloseTimerStarted bool
	closeTimer                   *time.Timer
	closeTimerStarted            bool

	// in
	extSourceSetReady    chan source.ExtSetReadyReq
	extSourceSetNotReady chan source.ExtSetNotReadyReq
	describeReq          chan readpublisher.DescribeReq
	setupPlayReq         chan readpublisher.SetupPlayReq
	announceReq          chan readpublisher.AnnounceReq
	playReq              chan readpublisher.PlayReq
	recordReq            chan readpublisher.RecordReq
	pauseReq             chan readpublisher.PauseReq
	removeReq            chan readpublisher.RemoveReq
	terminate            chan struct{}
}

// New allocates a Path.
func New(
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	confName string,
	conf *conf.PathConf,
	name string,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Path {

	pa := &Path{
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
		readPublishers:        make(map[readpublisher.ReadPublisher]readPublisherState),
		readers:               newReadersMap(),
		describeTimer:         newEmptyTimer(),
		sourceCloseTimer:      newEmptyTimer(),
		runOnDemandCloseTimer: newEmptyTimer(),
		closeTimer:            newEmptyTimer(),
		extSourceSetReady:     make(chan source.ExtSetReadyReq),
		extSourceSetNotReady:  make(chan source.ExtSetNotReadyReq),
		describeReq:           make(chan readpublisher.DescribeReq),
		setupPlayReq:          make(chan readpublisher.SetupPlayReq),
		announceReq:           make(chan readpublisher.AnnounceReq),
		playReq:               make(chan readpublisher.PlayReq),
		recordReq:             make(chan readpublisher.RecordReq),
		pauseReq:              make(chan readpublisher.PauseReq),
		removeReq:             make(chan readpublisher.RemoveReq),
		terminate:             make(chan struct{}),
	}

	pa.wg.Add(1)
	go pa.run()
	return pa
}

// Close closes a path.
func (pa *Path) Close() {
	close(pa.terminate)
}

// Log is the main logging function.
func (pa *Path) Log(level logger.Level, format string, args ...interface{}) {
	pa.parent.Log(level, "[path "+pa.name+"] "+format, args...)
}

func (pa *Path) run() {
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
				req.Res <- readpublisher.DescribeRes{nil, "", fmt.Errorf("publisher of path '%s' has timed out", pa.name)} //nolint:govet
			}
			pa.describeRequests = nil

			for _, req := range pa.setupPlayRequests {
				req.Res <- readpublisher.SetupPlayRes{nil, nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name)} //nolint:govet
			}
			pa.setupPlayRequests = nil

			// set state after removeReadPublisher(), so schedule* works once
			pa.sourceState = sourceStateNotReady

			pa.scheduleSourceClose()
			pa.scheduleRunOnDemandClose()
			pa.scheduleClose()

		case <-pa.sourceCloseTimer.C:
			pa.sourceCloseTimerStarted = false
			pa.source.(source.ExtSource).Close()
			pa.source = nil

			pa.scheduleClose()

		case <-pa.runOnDemandCloseTimer.C:
			pa.runOnDemandCloseTimerStarted = false
			pa.Log(logger.Info, "on demand command stopped")
			pa.onDemandCmd.Close()
			pa.onDemandCmd = nil

			pa.scheduleClose()

		case <-pa.closeTimer.C:
			pa.exhaustChannels()
			pa.parent.OnPathClose(pa)
			<-pa.terminate
			break outer

		case req := <-pa.extSourceSetReady:
			pa.sourceTracks = req.Tracks
			pa.sp = streamproc.New(pa, len(req.Tracks))
			pa.onSourceSetReady()
			req.Res <- source.ExtSetReadyRes{SP: pa.sp}

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

			if pa.readPublishers[req.Author] != readPublisherStatePreRemove {
				pa.removeReadPublisher(req.Author)
			}

			delete(pa.readPublishers, req.Author)
			pa.readPublishersWg.Done()
			close(req.Res)

		case <-pa.terminate:
			pa.exhaustChannels()
			break outer
		}
	}

	pa.describeTimer.Stop()
	pa.sourceCloseTimer.Stop()
	pa.runOnDemandCloseTimer.Stop()
	pa.closeTimer.Stop()

	if onInitCmd != nil {
		pa.Log(logger.Info, "on init command stopped")
		onInitCmd.Close()
	}

	if source, ok := pa.source.(source.ExtSource); ok {
		source.Close()
	}
	pa.sourceWg.Wait()

	if pa.onDemandCmd != nil {
		pa.Log(logger.Info, "on demand command stopped")
		pa.onDemandCmd.Close()
	}

	for _, req := range pa.describeRequests {
		req.Res <- readpublisher.DescribeRes{nil, "", fmt.Errorf("terminated")} //nolint:govet
	}

	for _, req := range pa.setupPlayRequests {
		req.Res <- readpublisher.SetupPlayRes{nil, nil, fmt.Errorf("terminated")} //nolint:govet
	}

	for c, state := range pa.readPublishers {
		if state != readPublisherStatePreRemove {
			switch state {
			case readPublisherStatePlay:
				atomic.AddInt64(pa.stats.CountReaders, -1)
				pa.readers.remove(c)

			case readPublisherStateRecord:
				atomic.AddInt64(pa.stats.CountPublishers, -1)
			}
			c.Close()
		}
	}
	pa.readPublishersWg.Wait()

	close(pa.extSourceSetReady)
	close(pa.extSourceSetNotReady)
	close(pa.describeReq)
	close(pa.setupPlayReq)
	close(pa.announceReq)
	close(pa.playReq)
	close(pa.recordReq)
	close(pa.pauseReq)
	close(pa.removeReq)
}

func (pa *Path) exhaustChannels() {
	go func() {
		for {
			select {
			case req, ok := <-pa.extSourceSetReady:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.extSourceSetNotReady:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.describeReq:
				if !ok {
					return
				}
				req.Res <- readpublisher.DescribeRes{nil, "", fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.setupPlayReq:
				if !ok {
					return
				}
				req.Res <- readpublisher.SetupPlayRes{nil, nil, fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.announceReq:
				if !ok {
					return
				}
				req.Res <- readpublisher.AnnounceRes{nil, fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.playReq:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.recordReq:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.pauseReq:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.removeReq:
				if !ok {
					return
				}

				if _, ok := pa.readPublishers[req.Author]; !ok {
					close(req.Res)
					continue
				}

				pa.readPublishersWg.Done()
				close(req.Res)
			}
		}
	}()
}

func (pa *Path) hasExternalSource() bool {
	return strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") ||
		strings.HasPrefix(pa.conf.Source, "rtmp://")
}

func (pa *Path) startExternalSource() {
	if strings.HasPrefix(pa.conf.Source, "rtsp://") ||
		strings.HasPrefix(pa.conf.Source, "rtsps://") {
		pa.source = rtspsource.New(
			pa.conf.Source,
			pa.conf.SourceProtocolParsed,
			pa.conf.SourceFingerprint,
			pa.readTimeout,
			pa.writeTimeout,
			pa.readBufferCount,
			pa.readBufferSize,
			&pa.sourceWg,
			pa.stats,
			pa)

	} else if strings.HasPrefix(pa.conf.Source, "rtmp://") {
		pa.source = rtmpsource.New(
			pa.conf.Source,
			pa.readTimeout,
			pa.writeTimeout,
			&pa.sourceWg,
			pa.stats,
			pa)
	}
}

func (pa *Path) hasReadPublishers() bool {
	for _, state := range pa.readPublishers {
		if state != readPublisherStatePreRemove {
			return true
		}
	}
	return false
}

func (pa *Path) hasReadPublishersNotSources() bool {
	for c, state := range pa.readPublishers {
		if state != readPublisherStatePreRemove && c != pa.source {
			return true
		}
	}
	return false
}

func (pa *Path) addReadPublisher(c readpublisher.ReadPublisher, state readPublisherState) {
	pa.readPublishers[c] = state
	pa.readPublishersWg.Add(1)
}

func (pa *Path) removeReadPublisher(c readpublisher.ReadPublisher) {
	state := pa.readPublishers[c]
	pa.readPublishers[c] = readPublisherStatePreRemove

	switch state {
	case readPublisherStatePlay:
		atomic.AddInt64(pa.stats.CountReaders, -1)
		pa.readers.remove(c)

	case readPublisherStateRecord:
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.onSourceSetNotReady()
	}

	if pa.source == c {
		pa.source = nil

		// close all readPublishers that are reading or waiting to read
		for oc, state := range pa.readPublishers {
			if state != readPublisherStatePreRemove {
				pa.removeReadPublisher(oc)
				oc.Close()
			}
		}
	}

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()
}

func (pa *Path) onSourceSetReady() {
	if pa.sourceState == sourceStateWaitingDescribe {
		pa.describeTimer.Stop()
		pa.describeTimer = newEmptyTimer()
	}

	pa.sourceState = sourceStateReady

	for _, req := range pa.describeRequests {
		req.Res <- readpublisher.DescribeRes{pa.sourceTracks.Write(), "", nil} //nolint:govet
	}
	pa.describeRequests = nil

	for _, req := range pa.setupPlayRequests {
		pa.onReadPublisherSetupPlayPost(req)
	}
	pa.setupPlayRequests = nil

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()
}

func (pa *Path) onSourceSetNotReady() {
	pa.sourceState = sourceStateNotReady

	// close all readPublishers that are reading or waiting to read
	for c, state := range pa.readPublishers {
		if c != pa.source && state != readPublisherStatePreRemove {
			pa.removeReadPublisher(c)
			c.Close()
		}
	}
}

func (pa *Path) fixedPublisherStart() {
	if pa.hasExternalSource() {
		// start on-demand source
		if pa.source == nil {
			pa.startExternalSource()

			if pa.sourceState != sourceStateWaitingDescribe {
				pa.describeTimer = time.NewTimer(pa.conf.SourceOnDemandStartTimeout)
				pa.sourceState = sourceStateWaitingDescribe
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

			if pa.sourceState != sourceStateWaitingDescribe {
				pa.describeTimer = time.NewTimer(pa.conf.RunOnDemandStartTimeout)
				pa.sourceState = sourceStateWaitingDescribe
			}

			// reset timer
		} else if pa.runOnDemandCloseTimerStarted {
			pa.runOnDemandCloseTimer.Stop()
			pa.runOnDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
		}
	}
}

func (pa *Path) onReadPublisherDescribe(req readpublisher.DescribeReq) {
	pa.fixedPublisherStart()
	pa.scheduleClose()

	if _, ok := pa.source.(*sourceRedirect); ok {
		req.Res <- readpublisher.DescribeRes{nil, pa.conf.SourceRedirect, nil} //nolint:govet
		return
	}

	switch pa.sourceState {
	case sourceStateReady:
		req.Res <- readpublisher.DescribeRes{pa.sourceTracks.Write(), "", nil} //nolint:govet
		return

	case sourceStateWaitingDescribe:
		pa.describeRequests = append(pa.describeRequests, req)
		return

	case sourceStateNotReady:
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
			req.Res <- readpublisher.DescribeRes{nil, fallbackURL, nil} //nolint:govet
			return
		}

		req.Res <- readpublisher.DescribeRes{nil, "", readpublisher.ErrNoOnePublishing{pa.name}} //nolint:govet
		return
	}
}

func (pa *Path) onReadPublisherSetupPlay(req readpublisher.SetupPlayReq) {
	pa.fixedPublisherStart()
	pa.scheduleClose()

	switch pa.sourceState {
	case sourceStateReady:
		pa.onReadPublisherSetupPlayPost(req)
		return

	case sourceStateWaitingDescribe:
		pa.setupPlayRequests = append(pa.setupPlayRequests, req)
		return

	case sourceStateNotReady:
		req.Res <- readpublisher.SetupPlayRes{nil, nil, readpublisher.ErrNoOnePublishing{pa.name}} //nolint:govet
		return
	}
}

func (pa *Path) onReadPublisherSetupPlayPost(req readpublisher.SetupPlayReq) {
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

		pa.addReadPublisher(req.Author, readPublisherStatePrePlay)
	}

	req.Res <- readpublisher.SetupPlayRes{pa, pa.sourceTracks, nil} //nolint:govet
}

func (pa *Path) onReadPublisherPlay(req readpublisher.PlayReq) {
	atomic.AddInt64(pa.stats.CountReaders, 1)
	pa.readPublishers[req.Author] = readPublisherStatePlay
	pa.readers.add(req.Author)

	req.Res <- readpublisher.PlayRes{TrackInfos: pa.sp.TrackInfos()}
}

func (pa *Path) onReadPublisherAnnounce(req readpublisher.AnnounceReq) {
	if _, ok := pa.readPublishers[req.Author]; ok {
		req.Res <- readpublisher.AnnounceRes{nil, fmt.Errorf("already publishing or reading")} //nolint:govet
		return
	}

	if pa.hasExternalSource() {
		req.Res <- readpublisher.AnnounceRes{nil, fmt.Errorf("path '%s' is assigned to an external source", pa.name)} //nolint:govet
		return
	}

	if pa.source != nil {
		if pa.conf.DisablePublisherOverride {
			req.Res <- readpublisher.AnnounceRes{nil, fmt.Errorf("another client is already publishing on path '%s'", pa.name)} //nolint:govet
			return
		}

		pa.Log(logger.Info, "closing existing publisher")
		curPublisher := pa.source.(readpublisher.ReadPublisher)
		pa.removeReadPublisher(curPublisher)
		curPublisher.Close()

		// prevent path closure
		if pa.closeTimerStarted {
			pa.closeTimer.Stop()
			pa.closeTimer = newEmptyTimer()
			pa.closeTimerStarted = false
		}
	}

	pa.addReadPublisher(req.Author, readPublisherStatePreRecord)

	pa.source = req.Author
	pa.sourceTracks = req.Tracks
	req.Res <- readpublisher.AnnounceRes{pa, nil} //nolint:govet
}

func (pa *Path) onReadPublisherRecord(req readpublisher.RecordReq) {
	if state, ok := pa.readPublishers[req.Author]; !ok || state != readPublisherStatePreRecord {
		req.Res <- readpublisher.RecordRes{SP: nil, Err: fmt.Errorf("not recording anymore")}
		return
	}

	atomic.AddInt64(pa.stats.CountPublishers, 1)
	pa.readPublishers[req.Author] = readPublisherStateRecord
	pa.onSourceSetReady()

	pa.sp = streamproc.New(pa, len(pa.sourceTracks))

	req.Res <- readpublisher.RecordRes{SP: pa.sp, Err: nil}
}

func (pa *Path) onReadPublisherPause(req readpublisher.PauseReq) {
	state, ok := pa.readPublishers[req.Author]
	if !ok {
		close(req.Res)
		return
	}

	if state == readPublisherStatePlay {
		atomic.AddInt64(pa.stats.CountReaders, -1)
		pa.readPublishers[req.Author] = readPublisherStatePrePlay
		pa.readers.remove(req.Author)

	} else if state == readPublisherStateRecord {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.readPublishers[req.Author] = readPublisherStatePreRecord
		pa.onSourceSetNotReady()
	}

	close(req.Res)
}

func (pa *Path) scheduleSourceClose() {
	if !pa.hasExternalSource() || !pa.conf.SourceOnDemand || pa.source == nil {
		return
	}

	if pa.sourceCloseTimerStarted ||
		pa.sourceState == sourceStateWaitingDescribe ||
		pa.hasReadPublishers() {
		return
	}

	pa.sourceCloseTimer.Stop()
	pa.sourceCloseTimer = time.NewTimer(pa.conf.SourceOnDemandCloseAfter)
	pa.sourceCloseTimerStarted = true
}

func (pa *Path) scheduleRunOnDemandClose() {
	if pa.conf.RunOnDemand == "" || pa.onDemandCmd == nil {
		return
	}

	if pa.runOnDemandCloseTimerStarted ||
		pa.sourceState == sourceStateWaitingDescribe ||
		pa.hasReadPublishersNotSources() {
		return
	}

	pa.runOnDemandCloseTimer.Stop()
	pa.runOnDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
	pa.runOnDemandCloseTimerStarted = true
}

func (pa *Path) scheduleClose() {
	if pa.conf.Regexp != nil &&
		!pa.hasReadPublishers() &&
		pa.source == nil &&
		pa.sourceState != sourceStateWaitingDescribe &&
		!pa.sourceCloseTimerStarted &&
		!pa.runOnDemandCloseTimerStarted &&
		!pa.closeTimerStarted {

		pa.closeTimer.Stop()
		pa.closeTimer = time.NewTimer(0)
		pa.closeTimerStarted = true
	}
}

// ConfName returns the configuration name of this path.
func (pa *Path) ConfName() string {
	return pa.confName
}

// Conf returns the configuration of this path.
func (pa *Path) Conf() *conf.PathConf {
	return pa.conf
}

// Name returns the name of this path.
func (pa *Path) Name() string {
	return pa.name
}

// OnExtSourceSetReady is called by an external source.
func (pa *Path) OnExtSourceSetReady(req source.ExtSetReadyReq) {
	pa.extSourceSetReady <- req
}

// OnExtSourceSetNotReady is called by an external source.
func (pa *Path) OnExtSourceSetNotReady(req source.ExtSetNotReadyReq) {
	pa.extSourceSetNotReady <- req
}

// OnPathManDescribe is called by pathman.PathMan.
func (pa *Path) OnPathManDescribe(req readpublisher.DescribeReq) {
	pa.describeReq <- req
}

// OnPathManSetupPlay is called by pathman.PathMan.
func (pa *Path) OnPathManSetupPlay(req readpublisher.SetupPlayReq) {
	pa.setupPlayReq <- req
}

// OnPathManAnnounce is called by pathman.PathMan.
func (pa *Path) OnPathManAnnounce(req readpublisher.AnnounceReq) {
	pa.announceReq <- req
}

// OnReadPublisherRemove is called by a readpublisher.
func (pa *Path) OnReadPublisherRemove(req readpublisher.RemoveReq) {
	pa.removeReq <- req
}

// OnReadPublisherPlay is called by a readpublisher.
func (pa *Path) OnReadPublisherPlay(req readpublisher.PlayReq) {
	pa.playReq <- req
}

// OnReadPublisherRecord is called by a readpublisher.
func (pa *Path) OnReadPublisherRecord(req readpublisher.RecordReq) {
	pa.recordReq <- req
}

// OnReadPublisherPause is called by a readpublisher.
func (pa *Path) OnReadPublisherPause(req readpublisher.PauseReq) {
	pa.pauseReq <- req
}

// OnSPFrame is called by streamproc.StreamProc.
func (pa *Path) OnSPFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	pa.readers.forwardFrame(trackID, streamType, payload)
}
