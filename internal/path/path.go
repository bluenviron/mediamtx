package path

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/externalcmd"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/sourcertmp"
	"github.com/aler9/rtsp-simple-server/internal/sourcertsp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
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
	OnPathClientClose(client.Client)
}

// source is implemented by all sources (client* and source*).
type source interface {
	IsSource()
}

// sourceExternal is implemented by all source*.
type sourceExternal interface {
	IsSource()
	IsSourceExternal()
	Close()
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
	rtspPort        int
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

	clients                      map[client.Client]clientState
	clientsWg                    sync.WaitGroup
	describeRequests             []client.DescribeReq
	setupPlayRequests            []client.SetupPlayReq
	source                       source
	sourceTracks                 gortsplib.Tracks
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
	sourceSetReady    chan struct{} // from source
	sourceSetNotReady chan struct{} // from source
	clientDescribe    chan client.DescribeReq
	clientSetupPlay   chan client.SetupPlayReq
	clientAnnounce    chan client.AnnounceReq
	clientPlay        chan client.PlayReq
	clientRecord      chan client.RecordReq
	clientPause       chan client.PauseReq
	clientRemove      chan client.RemoveReq
	terminate         chan struct{}
}

// New allocates a Path.
func New(
	rtspPort int,
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
		rtspPort:              rtspPort,
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
		clients:               make(map[client.Client]clientState),
		readers:               newReadersMap(),
		describeTimer:         newEmptyTimer(),
		sourceCloseTimer:      newEmptyTimer(),
		runOnDemandCloseTimer: newEmptyTimer(),
		closeTimer:            newEmptyTimer(),
		sourceSetReady:        make(chan struct{}),
		sourceSetNotReady:     make(chan struct{}),
		clientDescribe:        make(chan client.DescribeReq),
		clientSetupPlay:       make(chan client.SetupPlayReq),
		clientAnnounce:        make(chan client.AnnounceReq),
		clientPlay:            make(chan client.PlayReq),
		clientRecord:          make(chan client.RecordReq),
		clientPause:           make(chan client.PauseReq),
		clientRemove:          make(chan client.RemoveReq),
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
		onInitCmd = externalcmd.New(pa.conf.RunOnInit, pa.conf.RunOnInitRestart, externalcmd.Environment{
			Path: pa.name,
			Port: strconv.FormatInt(int64(pa.rtspPort), 10),
		})
	}

outer:
	for {
		select {
		case <-pa.describeTimer.C:
			for _, req := range pa.describeRequests {
				req.Res <- client.DescribeRes{nil, "", fmt.Errorf("publisher of path '%s' has timed out", pa.name)} //nolint:govet
			}
			pa.describeRequests = nil

			for _, req := range pa.setupPlayRequests {
				req.Res <- client.SetupPlayRes{nil, nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name)} //nolint:govet
			}
			pa.setupPlayRequests = nil

			// set state after removeClient(), so schedule* works once
			pa.sourceState = sourceStateNotReady

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
			pa.exhaustChannels()
			pa.parent.OnPathClose(pa)
			<-pa.terminate
			break outer

		case <-pa.sourceSetReady:
			pa.onSourceSetReady()

		case <-pa.sourceSetNotReady:
			pa.onSourceSetNotReady()

		case req := <-pa.clientDescribe:
			pa.onClientDescribe(req)

		case req := <-pa.clientSetupPlay:
			pa.onClientSetupPlay(req)

		case req := <-pa.clientAnnounce:
			pa.onClientAnnounce(req)

		case req := <-pa.clientPlay:
			pa.onClientPlay(req.Client)
			close(req.Res)

		case req := <-pa.clientRecord:
			pa.onClientRecord(req.Client)
			close(req.Res)

		case req := <-pa.clientPause:
			pa.onClientPause(req.Client)
			close(req.Res)

		case req := <-pa.clientRemove:
			if _, ok := pa.clients[req.Client]; !ok {
				close(req.Res)
				continue
			}

			if pa.clients[req.Client] != clientStatePreRemove {
				pa.removeClient(req.Client)
			}

			delete(pa.clients, req.Client)
			pa.clientsWg.Done()

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

	if source, ok := pa.source.(sourceExternal); ok {
		source.Close()
	}
	pa.sourceWg.Wait()

	if pa.onDemandCmd != nil {
		pa.Log(logger.Info, "on demand command stopped")
		pa.onDemandCmd.Close()
	}

	for _, req := range pa.describeRequests {
		req.Res <- client.DescribeRes{nil, "", fmt.Errorf("terminated")} //nolint:govet
	}

	for _, req := range pa.setupPlayRequests {
		req.Res <- client.SetupPlayRes{nil, nil, fmt.Errorf("terminated")} //nolint:govet
	}

	for c, state := range pa.clients {
		if state != clientStatePreRemove {
			switch state {
			case clientStatePlay:
				atomic.AddInt64(pa.stats.CountReaders, -1)
				pa.readers.remove(c)

			case clientStateRecord:
				atomic.AddInt64(pa.stats.CountPublishers, -1)
			}
			pa.parent.OnPathClientClose(c)
		}
	}
	pa.clientsWg.Wait()

	close(pa.sourceSetReady)
	close(pa.sourceSetNotReady)
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
			case _, ok := <-pa.sourceSetReady:
				if !ok {
					return
				}

			case _, ok := <-pa.sourceSetNotReady:
				if !ok {
					return
				}

			case req, ok := <-pa.clientDescribe:
				if !ok {
					return
				}
				req.Res <- client.DescribeRes{nil, "", fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.clientSetupPlay:
				if !ok {
					return
				}
				req.Res <- client.SetupPlayRes{nil, nil, fmt.Errorf("terminated")} //nolint:govet

			case req, ok := <-pa.clientAnnounce:
				if !ok {
					return
				}
				req.Res <- client.AnnounceRes{nil, fmt.Errorf("terminated")} //nolint:govet

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

				if _, ok := pa.clients[req.Client]; !ok {
					close(req.Res)
					continue
				}

				pa.clientsWg.Done()

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
			&pa.sourceWg,
			pa.stats,
			pa)
	}
}

func (pa *Path) hasClients() bool {
	for _, state := range pa.clients {
		if state != clientStatePreRemove {
			return true
		}
	}
	return false
}

func (pa *Path) hasClientsNotSources() bool {
	for c, state := range pa.clients {
		if state != clientStatePreRemove && c != pa.source {
			return true
		}
	}
	return false
}

func (pa *Path) addClient(c client.Client, state clientState) {
	pa.clients[c] = state
	pa.clientsWg.Add(1)
}

func (pa *Path) removeClient(c client.Client) {
	state := pa.clients[c]
	pa.clients[c] = clientStatePreRemove

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

		// close all clients that are reading or waiting to read
		for oc, state := range pa.clients {
			if state != clientStatePreRemove {
				pa.removeClient(oc)
				pa.parent.OnPathClientClose(oc)
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
		req.Res <- client.DescribeRes{pa.sourceTracks.Write(), "", nil} //nolint:govet
	}
	pa.describeRequests = nil

	for _, req := range pa.setupPlayRequests {
		pa.onClientSetupPlayPost(req)
	}
	pa.setupPlayRequests = nil

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()
}

func (pa *Path) onSourceSetNotReady() {
	pa.sourceState = sourceStateNotReady

	// close all clients that are reading or waiting to read
	for c, state := range pa.clients {
		if c != pa.source && state != clientStatePreRemove {
			pa.removeClient(c)
			pa.parent.OnPathClientClose(c)
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

		} else {
			// reset timer
			if pa.sourceCloseTimerStarted {
				pa.sourceCloseTimer.Stop()
				pa.sourceCloseTimer = time.NewTimer(pa.conf.SourceOnDemandCloseAfter)
			}
		}
	}

	if pa.conf.RunOnDemand != "" {
		// start on-demand command
		if pa.onDemandCmd == nil {
			pa.Log(logger.Info, "on demand command started")
			pa.onDemandCmd = externalcmd.New(pa.conf.RunOnDemand, pa.conf.RunOnDemandRestart, externalcmd.Environment{
				Path: pa.name,
				Port: strconv.FormatInt(int64(pa.rtspPort), 10),
			})

			if pa.sourceState != sourceStateWaitingDescribe {
				pa.describeTimer = time.NewTimer(pa.conf.RunOnDemandStartTimeout)
				pa.sourceState = sourceStateWaitingDescribe
			}

		} else {
			// reset timer
			if pa.runOnDemandCloseTimerStarted {
				pa.runOnDemandCloseTimer.Stop()
				pa.runOnDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
			}
		}
	}
}

func (pa *Path) onClientDescribe(req client.DescribeReq) {
	if _, ok := pa.clients[req.Client]; ok {
		req.Res <- client.DescribeRes{nil, "", fmt.Errorf("already subscribed")} //nolint:govet
		return
	}

	pa.fixedPublisherStart()
	pa.scheduleClose()

	if _, ok := pa.source.(*sourceRedirect); ok {
		req.Res <- client.DescribeRes{nil, pa.conf.SourceRedirect, nil} //nolint:govet
		return
	}

	switch pa.sourceState {
	case sourceStateReady:
		req.Res <- client.DescribeRes{pa.sourceTracks.Write(), "", nil} //nolint:govet
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
			req.Res <- client.DescribeRes{nil, fallbackURL, nil} //nolint:govet
			return
		}

		req.Res <- client.DescribeRes{nil, "", client.ErrNoOnePublishing{pa.name}} //nolint:govet
		return
	}
}

func (pa *Path) onClientSetupPlay(req client.SetupPlayReq) {
	pa.fixedPublisherStart()
	pa.scheduleClose()

	switch pa.sourceState {
	case sourceStateReady:
		pa.onClientSetupPlayPost(req)
		return

	case sourceStateWaitingDescribe:
		pa.setupPlayRequests = append(pa.setupPlayRequests, req)
		return

	case sourceStateNotReady:
		req.Res <- client.SetupPlayRes{nil, nil, client.ErrNoOnePublishing{pa.name}} //nolint:govet
		return
	}
}

func (pa *Path) onClientSetupPlayPost(req client.SetupPlayReq) {
	if _, ok := pa.clients[req.Client]; !ok {
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

		pa.addClient(req.Client, clientStatePrePlay)
	}

	req.Res <- client.SetupPlayRes{pa, pa.sourceTracks, nil} //nolint:govet
}

func (pa *Path) onClientPlay(c client.Client) {
	state, ok := pa.clients[c]
	if !ok {
		return
	}

	if state != clientStatePrePlay {
		return
	}

	atomic.AddInt64(pa.stats.CountReaders, 1)
	pa.clients[c] = clientStatePlay
	pa.readers.add(c)
}

func (pa *Path) onClientAnnounce(req client.AnnounceReq) {
	if _, ok := pa.clients[req.Client]; ok {
		req.Res <- client.AnnounceRes{nil, fmt.Errorf("already publishing or reading")} //nolint:govet
		return
	}

	if pa.hasExternalSource() {
		req.Res <- client.AnnounceRes{nil, fmt.Errorf("path '%s' is assigned to an external source", pa.name)} //nolint:govet
		return
	}

	if pa.source != nil {
		pa.Log(logger.Info, "disconnecting existing publisher")
		curPublisher := pa.source.(client.Client)
		pa.removeClient(curPublisher)
		pa.parent.OnPathClientClose(curPublisher)

		// prevent path closure
		if pa.closeTimerStarted {
			pa.closeTimer.Stop()
			pa.closeTimer = newEmptyTimer()
			pa.closeTimerStarted = false
		}
	}

	pa.addClient(req.Client, clientStatePreRecord)

	pa.source = req.Client
	pa.sourceTracks = req.Tracks
	req.Res <- client.AnnounceRes{pa, nil} //nolint:govet
}

func (pa *Path) onClientRecord(c client.Client) {
	state, ok := pa.clients[c]
	if !ok {
		return
	}

	if state != clientStatePreRecord {
		return
	}

	atomic.AddInt64(pa.stats.CountPublishers, 1)
	pa.clients[c] = clientStateRecord
	pa.onSourceSetReady()
}

func (pa *Path) onClientPause(c client.Client) {
	state, ok := pa.clients[c]
	if !ok {
		return
	}

	if state == clientStatePlay {
		atomic.AddInt64(pa.stats.CountReaders, -1)
		pa.clients[c] = clientStatePrePlay
		pa.readers.remove(c)

	} else if state == clientStateRecord {
		atomic.AddInt64(pa.stats.CountPublishers, -1)
		pa.clients[c] = clientStatePreRecord
		pa.onSourceSetNotReady()
	}
}

func (pa *Path) scheduleSourceClose() {
	if !pa.hasExternalSource() || !pa.conf.SourceOnDemand || pa.source == nil {
		return
	}

	if pa.sourceCloseTimerStarted ||
		pa.sourceState == sourceStateWaitingDescribe ||
		pa.hasClients() {
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
		pa.hasClientsNotSources() {
		return
	}

	pa.runOnDemandCloseTimer.Stop()
	pa.runOnDemandCloseTimer = time.NewTimer(pa.conf.RunOnDemandCloseAfter)
	pa.runOnDemandCloseTimerStarted = true
}

func (pa *Path) scheduleClose() {
	if pa.conf.Regexp != nil &&
		!pa.hasClients() &&
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

// OnSourceSetReady is called by a source.
func (pa *Path) OnSourceSetReady(tracks gortsplib.Tracks) {
	pa.sourceTracks = tracks
	pa.sourceSetReady <- struct{}{}
}

// OnSourceSetNotReady is called by a source.
func (pa *Path) OnSourceSetNotReady() {
	pa.sourceSetNotReady <- struct{}{}
}

// OnPathManDescribe is called by pathman.PathMan.
func (pa *Path) OnPathManDescribe(req client.DescribeReq) {
	pa.clientDescribe <- req
}

// OnPathManSetupPlay is called by pathman.PathMan.
func (pa *Path) OnPathManSetupPlay(req client.SetupPlayReq) {
	pa.clientSetupPlay <- req
}

// OnPathManAnnounce is called by pathman.PathMan.
func (pa *Path) OnPathManAnnounce(req client.AnnounceReq) {
	pa.clientAnnounce <- req
}

// OnClientRemove is called by clientrtsp.Client.
func (pa *Path) OnClientRemove(req client.RemoveReq) {
	pa.clientRemove <- req
}

// OnClientPlay is called by clientrtsp.Client.
func (pa *Path) OnClientPlay(req client.PlayReq) {
	pa.clientPlay <- req
}

// OnClientRecord is called by clientrtsp.Client.
func (pa *Path) OnClientRecord(req client.RecordReq) {
	pa.clientRecord <- req
}

// OnClientPause is called by clientrtsp.Client.
func (pa *Path) OnClientPause(req client.PauseReq) {
	pa.clientPause <- req
}

// OnFrame is called by a source or by a clientrtsp.Client.
func (pa *Path) OnFrame(trackID int, streamType gortsplib.StreamType, buf []byte) {
	pa.readers.forwardFrame(trackID, streamType, buf)
}
