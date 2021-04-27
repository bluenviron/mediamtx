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
	"github.com/aler9/rtsp-simple-server/internal/sourcertmp"
	"github.com/aler9/rtsp-simple-server/internal/sourcertsp"
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

type clientState int

const (
	clientStatePrePlay clientState = iota
	clientStatePlay
	clientStatePreRecord
	clientStateRecord
	clientStatePreRemove
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

	readPublishers               map[readpublisher.ReadPublisher]clientState
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
	clientDescribe       chan readpublisher.DescribeReq
	clientSetupPlay      chan readpublisher.SetupPlayReq
	clientAnnounce       chan readpublisher.AnnounceReq
	clientPlay           chan readpublisher.PlayReq
	clientRecord         chan readpublisher.RecordReq
	clientPause          chan readpublisher.PauseReq
	clientRemove         chan readpublisher.RemoveReq
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
		readPublishers:        make(map[readpublisher.ReadPublisher]clientState),
		readers:               newReadersMap(),
		describeTimer:         newEmptyTimer(),
		sourceCloseTimer:      newEmptyTimer(),
		runOnDemandCloseTimer: newEmptyTimer(),
		closeTimer:            newEmptyTimer(),
		extSourceSetReady:     make(chan source.ExtSetReadyReq),
		extSourceSetNotReady:  make(chan source.ExtSetNotReadyReq),
		clientDescribe:        make(chan readpublisher.DescribeReq),
		clientSetupPlay:       make(chan readpublisher.SetupPlayReq),
		clientAnnounce:        make(chan readpublisher.AnnounceReq),
		clientPlay:            make(chan readpublisher.PlayReq),
		clientRecord:          make(chan readpublisher.RecordReq),
		clientPause:           make(chan readpublisher.PauseReq),
		clientRemove:          make(chan readpublisher.RemoveReq),
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

		case req := <-pa.clientDescribe:
			pa.onReadPublisherDescribe(req)

		case req := <-pa.clientSetupPlay:
			pa.onReadPublisherSetupPlay(req)

		case req := <-pa.clientAnnounce:
			pa.onReadPublisherAnnounce(req)

		case req := <-pa.clientPlay:
			pa.onReadPublisherPlay(req)

		case req := <-pa.clientRecord:
			pa.onReadPublisherRecord(req)

		case req := <-pa.clientPause:
			pa.onReadPublisherPause(req)

		case req := <-pa.clientRemove:
			if _, ok := pa.readPublishers[req.ReadPublisher]; !ok {
				close(req.Res)
				continue
			}

			if pa.readPublishers[req.ReadPublisher] != clientStatePreRemove {
				pa.removeReadPublisher(req.ReadPublisher)
			}

			delete(pa.readPublishers, req.ReadPublisher)
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
		if state != clientStatePreRemove {
			switch state {
			case clientStatePlay:
				atomic.AddInt64(pa.stats.CountReaders, -1)
				pa.readers.remove(c)

			case clientStateRecord:
				atomic.AddInt64(pa.stats.CountPublishers, -1)
			}
			c.CloseRequest()
		}
	}
	pa.readPublishersWg.Wait()

	close(pa.extSourceSetReady)
	close(pa.extSourceSetNotReady)
	close(pa.clientDescribe)
	close(pa.clientSetupPlay)
	close(pa.clientAnnounce)
	close(pa.clientPlay)
	close(pa.clientRecord)
	close(pa.clientPause)
	close(pa.clientRemove)
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

			case req, ok := <-pa.clientDescribe:
				if !ok {
					return
				}
				req.Res <- readpublisher.DescribeRes{nil, "", fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.clientSetupPlay:
				if !ok {
					return
				}
				req.Res <- readpublisher.SetupPlayRes{nil, nil, fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.clientAnnounce:
				if !ok {
					return
				}
				req.Res <- readpublisher.AnnounceRes{nil, fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.clientPlay:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.clientRecord:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.clientPause:
				if !ok {
					return
				}
				close(req.Res)

			case req, ok := <-pa.clientRemove:
				if !ok {
					return
				}

				if _, ok := pa.readPublishers[req.ReadPublisher]; !ok {
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
		pa.source = sourcertsp.New(
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
		pa.source = sourcertmp.New(
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
		if state != clientStatePreRemove {
			return true
		}
	}
	return false
}

func (pa *Path) hasReadPublishersNotSources() bool {
	for c, state := range pa.readPublishers {
		if state != clientStatePreRemove && c != pa.source {
			return true
		}
	}
	return false
}

func (pa *Path) addReadPublisher(c readpublisher.ReadPublisher, state clientState) {
	pa.readPublishers[c] = state
	pa.readPublishersWg.Add(1)
}

func (pa *Path) removeReadPublisher(c readpublisher.ReadPublisher) {
	state := pa.readPublishers[c]
	pa.readPublishers[c] = clientStatePreRemove

	switch state {
	case clientStatePlay:
		atomic.AddInt64(pa.stats.CountReaders, -1)
		pa.readers.remove(c)

	case clientStateRecord:
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.onSourceSetNotReady()
	}

	if pa.source == c {
		pa.source = nil

		// close all readPublishers that are reading or waiting to read
		for oc, state := range pa.readPublishers {
			if state != clientStatePreRemove {
				pa.removeReadPublisher(oc)
				oc.CloseRequest()
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
		if c != pa.source && state != clientStatePreRemove {
			pa.removeReadPublisher(c)
			c.CloseRequest()
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
	if _, ok := pa.readPublishers[req.ReadPublisher]; ok {
		req.Res <- readpublisher.DescribeRes{nil, "", fmt.Errorf("already subscribed")} //nolint:govet
		return
	}

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
						Scheme: req.Data.URL.Scheme,
						User:   req.Data.URL.User,
						Host:   req.Data.URL.Host,
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
	if _, ok := pa.readPublishers[req.ReadPublisher]; !ok {
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

		pa.addReadPublisher(req.ReadPublisher, clientStatePrePlay)
	}

	req.Res <- readpublisher.SetupPlayRes{pa, pa.sourceTracks, nil} //nolint:govet
}

func (pa *Path) onReadPublisherPlay(req readpublisher.PlayReq) {
	atomic.AddInt64(pa.stats.CountReaders, 1)
	pa.readPublishers[req.ReadPublisher] = clientStatePlay
	pa.readers.add(req.ReadPublisher)

	req.Res <- readpublisher.PlayRes{TrackInfos: pa.sp.TrackInfos()}
}

func (pa *Path) onReadPublisherAnnounce(req readpublisher.AnnounceReq) {
	if _, ok := pa.readPublishers[req.ReadPublisher]; ok {
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

		pa.Log(logger.Info, "disconnecting existing publisher")
		curPublisher := pa.source.(readpublisher.ReadPublisher)
		pa.removeReadPublisher(curPublisher)
		curPublisher.CloseRequest()

		// prevent path closure
		if pa.closeTimerStarted {
			pa.closeTimer.Stop()
			pa.closeTimer = newEmptyTimer()
			pa.closeTimerStarted = false
		}
	}

	pa.addReadPublisher(req.ReadPublisher, clientStatePreRecord)

	pa.source = req.ReadPublisher
	pa.sourceTracks = req.Tracks
	req.Res <- readpublisher.AnnounceRes{pa, nil} //nolint:govet
}

func (pa *Path) onReadPublisherRecord(req readpublisher.RecordReq) {
	if state, ok := pa.readPublishers[req.ReadPublisher]; !ok || state != clientStatePreRecord {
		req.Res <- readpublisher.RecordRes{SP: nil, Err: fmt.Errorf("not recording anymore")}
		return
	}

	atomic.AddInt64(pa.stats.CountPublishers, 1)
	pa.readPublishers[req.ReadPublisher] = clientStateRecord
	pa.onSourceSetReady()

	pa.sp = streamproc.New(pa, len(pa.sourceTracks))

	req.Res <- readpublisher.RecordRes{SP: pa.sp, Err: nil}
}

func (pa *Path) onReadPublisherPause(req readpublisher.PauseReq) {
	state, ok := pa.readPublishers[req.ReadPublisher]
	if !ok {
		close(req.Res)
		return
	}

	if state == clientStatePlay {
		atomic.AddInt64(pa.stats.CountReaders, -1)
		pa.readPublishers[req.ReadPublisher] = clientStatePrePlay
		pa.readers.remove(req.ReadPublisher)

	} else if state == clientStateRecord {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.readPublishers[req.ReadPublisher] = clientStatePreRecord
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
	pa.clientDescribe <- req
}

// OnPathManSetupPlay is called by pathman.PathMan.
func (pa *Path) OnPathManSetupPlay(req readpublisher.SetupPlayReq) {
	pa.clientSetupPlay <- req
}

// OnPathManAnnounce is called by pathman.PathMan.
func (pa *Path) OnPathManAnnounce(req readpublisher.AnnounceReq) {
	pa.clientAnnounce <- req
}

// OnReadPublisherRemove is called by a readpublisher.
func (pa *Path) OnReadPublisherRemove(req readpublisher.RemoveReq) {
	pa.clientRemove <- req
}

// OnReadPublisherPlay is called by a readpublisher.
func (pa *Path) OnReadPublisherPlay(req readpublisher.PlayReq) {
	pa.clientPlay <- req
}

// OnReadPublisherRecord is called by a readpublisher.
func (pa *Path) OnReadPublisherRecord(req readpublisher.RecordReq) {
	pa.clientRecord <- req
}

// OnReadPublisherPause is called by a readpublisher.
func (pa *Path) OnReadPublisherPause(req readpublisher.PauseReq) {
	pa.clientPause <- req
}

// OnSPFrame is called by streamproc.StreamProc.
func (pa *Path) OnSPFrame(trackID int, streamType gortsplib.StreamType, payload []byte) {
	pa.readers.forwardFrame(trackID, streamType, payload)
}
