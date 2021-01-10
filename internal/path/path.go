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
	OnPathClientClose(*client.Client)
}

// a source can be
// * client.Client
// * sourcertsp.Source
// * sourcertmp.Source
// * sourceRedirect
type source interface {
	IsSource()
}

// a sourceExternal can be
// * sourcertsp.Source
// * sourcertmp.Source
type sourceExternal interface {
	IsSource()
	IsSourceExternal()
	Close()
}

type sourceRedirect struct{}

func (*sourceRedirect) IsSource() {}

// ClientDescribeRes is a client describe response.
type ClientDescribeRes struct {
	Path client.Path
	Err  error
}

// ClientDescribeReq is a client describe request.
type ClientDescribeReq struct {
	Res      chan ClientDescribeRes
	Client   *client.Client
	PathName string
	Req      *base.Request
}

// ClientAnnounceRes is a client announce response.
type ClientAnnounceRes struct {
	Path client.Path
	Err  error
}

// ClientAnnounceReq is a client announce request.
type ClientAnnounceReq struct {
	Res      chan ClientAnnounceRes
	Client   *client.Client
	PathName string
	Tracks   gortsplib.Tracks
	Req      *base.Request
}

// ClientSetupPlayRes is a setup/play response.
type ClientSetupPlayRes struct {
	Path client.Path
	Err  error
}

// ClientSetupPlayReq is a setup/play request.
type ClientSetupPlayReq struct {
	Res      chan ClientSetupPlayRes
	Client   *client.Client
	PathName string
	TrackID  int
	Req      *base.Request
}

type clientRemoveReq struct {
	res    chan struct{}
	client *client.Client
}

type clientPlayReq struct {
	res    chan struct{}
	client *client.Client
}

type clientRecordReq struct {
	res    chan struct{}
	client *client.Client
}

type clientPauseReq struct {
	res    chan struct{}
	client *client.Client
}

type clientState int

const (
	clientStateWaitingDescribe clientState = iota
	clientStatePrePlay
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
	readBufferCount uint64
	confName        string
	conf            *conf.PathConf
	name            string
	wg              *sync.WaitGroup
	stats           *stats.Stats
	parent          Parent

	clients                      map[*client.Client]clientState
	clientsWg                    sync.WaitGroup
	source                       source
	sourceTrackCount             int
	sourceSdp                    []byte
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
	sourceSetReady    chan struct{}           // from source
	sourceSetNotReady chan struct{}           // from source
	clientDescribe    chan ClientDescribeReq  // from program
	clientAnnounce    chan ClientAnnounceReq  // from program
	clientSetupPlay   chan ClientSetupPlayReq // from program
	clientPlay        chan clientPlayReq      // from client
	clientRecord      chan clientRecordReq    // from client
	clientPause       chan clientPauseReq     // from client
	clientRemove      chan clientRemoveReq    // from client
	terminate         chan struct{}
}

// New allocates a Path.
func New(
	rtspPort int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount uint64,
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
		confName:              confName,
		conf:                  conf,
		name:                  name,
		wg:                    wg,
		stats:                 stats,
		parent:                parent,
		clients:               make(map[*client.Client]clientState),
		readers:               newReadersMap(),
		describeTimer:         newEmptyTimer(),
		sourceCloseTimer:      newEmptyTimer(),
		runOnDemandCloseTimer: newEmptyTimer(),
		closeTimer:            newEmptyTimer(),
		sourceSetReady:        make(chan struct{}),
		sourceSetNotReady:     make(chan struct{}),
		clientDescribe:        make(chan ClientDescribeReq),
		clientAnnounce:        make(chan ClientAnnounceReq),
		clientSetupPlay:       make(chan ClientSetupPlayReq),
		clientPlay:            make(chan clientPlayReq),
		clientRecord:          make(chan clientRecordReq),
		clientPause:           make(chan clientPauseReq),
		clientRemove:          make(chan clientRemoveReq),
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
			for c, state := range pa.clients {
				if state == clientStateWaitingDescribe {
					pa.removeClient(c)
					c.OnPathDescribeData(nil, "", fmt.Errorf("publisher of path '%s' has timed out", pa.name))
				}
			}

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
			if _, ok := pa.clients[req.Client]; ok {
				req.Res <- ClientDescribeRes{nil, fmt.Errorf("already subscribed")}
				continue
			}

			// reply immediately
			req.Res <- ClientDescribeRes{pa, nil}

			pa.onClientDescribe(req.Client)

		case req := <-pa.clientSetupPlay:
			err := pa.onClientSetupPlay(req.Client, req.TrackID)
			if err != nil {
				req.Res <- ClientSetupPlayRes{nil, err}
				continue
			}
			req.Res <- ClientSetupPlayRes{pa, nil}

		case req := <-pa.clientPlay:
			pa.onClientPlay(req.client)
			close(req.res)

		case req := <-pa.clientAnnounce:
			err := pa.onClientAnnounce(req.Client, req.Tracks)
			if err != nil {
				req.Res <- ClientAnnounceRes{nil, err}
				continue
			}
			req.Res <- ClientAnnounceRes{pa, nil}

		case req := <-pa.clientRecord:
			pa.onClientRecord(req.client)
			close(req.res)

		case req := <-pa.clientPause:
			pa.onClientPause(req.client)
			close(req.res)

		case req := <-pa.clientRemove:
			if _, ok := pa.clients[req.client]; !ok {
				close(req.res)
				continue
			}

			if pa.clients[req.client] != clientStatePreRemove {
				pa.removeClient(req.client)
			}

			delete(pa.clients, req.client)
			pa.clientsWg.Done()

			close(req.res)

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
	close(pa.clientAnnounce)
	close(pa.clientSetupPlay)
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
				req.Res <- ClientDescribeRes{nil, fmt.Errorf("terminated")}

			case req, ok := <-pa.clientAnnounce:
				if !ok {
					return
				}
				req.Res <- ClientAnnounceRes{nil, fmt.Errorf("terminated")}

			case req, ok := <-pa.clientSetupPlay:
				if !ok {
					return
				}
				req.Res <- ClientSetupPlayRes{nil, fmt.Errorf("terminated")}

			case req, ok := <-pa.clientPlay:
				if !ok {
					return
				}
				close(req.res)

			case req, ok := <-pa.clientRecord:
				if !ok {
					return
				}
				close(req.res)

			case req, ok := <-pa.clientPause:
				if !ok {
					return
				}
				close(req.res)

			case req, ok := <-pa.clientRemove:
				if !ok {
					return
				}

				if _, ok := pa.clients[req.client]; !ok {
					close(req.res)
					continue
				}

				pa.clientsWg.Done()

				close(req.res)
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
			&pa.sourceWg,
			pa.stats,
			pa)

	} else if strings.HasPrefix(pa.conf.Source, "rtmp://") {
		pa.source = sourcertmp.New(
			pa.conf.Source,
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

func (pa *Path) addClient(c *client.Client, state clientState) {
	if _, ok := pa.clients[c]; ok {
		panic("client already added")
	}

	pa.clients[c] = state
	pa.clientsWg.Add(1)
}

func (pa *Path) removeClient(c *client.Client) {
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
			if state != clientStatePreRemove && state != clientStateWaitingDescribe {
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

	// reply to all clients that are waiting for a description
	for c, state := range pa.clients {
		if state == clientStateWaitingDescribe {
			pa.removeClient(c)
			c.OnPathDescribeData(pa.sourceSdp, "", nil)
		}
	}

	pa.scheduleSourceClose()
	pa.scheduleRunOnDemandClose()
	pa.scheduleClose()
}

func (pa *Path) onSourceSetNotReady() {
	pa.sourceState = sourceStateNotReady

	// close all clients that are reading or waiting to read
	for c, state := range pa.clients {
		if state == clientStateWaitingDescribe {
			panic("not possible")
		}
		if c != pa.source && state != clientStatePreRemove {
			pa.removeClient(c)
			pa.parent.OnPathClientClose(c)
		}
	}
}

func (pa *Path) onClientDescribe(c *client.Client) {
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

	// start on-demand source
	if pa.hasExternalSource() {
		if pa.source == nil {
			pa.startExternalSource()

			if pa.sourceState != sourceStateWaitingDescribe {
				pa.describeTimer = time.NewTimer(pa.conf.SourceOnDemandStartTimeout)
				pa.sourceState = sourceStateWaitingDescribe
			}
		}
	}

	// start on-demand command
	if pa.conf.RunOnDemand != "" {
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
		}
	}

	if _, ok := pa.source.(*sourceRedirect); ok {
		pa.addClient(c, clientStatePreRemove)
		pa.removeClient(c)
		c.OnPathDescribeData(nil, pa.conf.SourceRedirect, nil)
		return
	}

	switch pa.sourceState {
	case sourceStateReady:
		pa.addClient(c, clientStatePreRemove)
		pa.removeClient(c)
		c.OnPathDescribeData(pa.sourceSdp, "", nil)
		return

	case sourceStateWaitingDescribe:
		pa.addClient(c, clientStateWaitingDescribe)
		return

	case sourceStateNotReady:
		if pa.conf.Fallback != "" {
			pa.addClient(c, clientStatePreRemove)
			pa.removeClient(c)
			c.OnPathDescribeData(nil, pa.conf.Fallback, nil)
			return
		}

		pa.addClient(c, clientStatePreRemove)
		pa.removeClient(c)
		c.OnPathDescribeData(nil, "", fmt.Errorf("no one is publishing to path '%s'", pa.name))
		return
	}
}

func (pa *Path) onClientSetupPlay(c *client.Client, trackID int) error {
	if pa.sourceState != sourceStateReady {
		return fmt.Errorf("no one is publishing to path '%s'", pa.name)
	}

	if trackID >= pa.sourceTrackCount {
		return fmt.Errorf("track %d does not exist", trackID)
	}

	if _, ok := pa.clients[c]; !ok {
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

		pa.addClient(c, clientStatePrePlay)
	}

	return nil
}

func (pa *Path) onClientPlay(c *client.Client) {
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

func (pa *Path) onClientAnnounce(c *client.Client, tracks gortsplib.Tracks) error {
	if _, ok := pa.clients[c]; ok {
		return fmt.Errorf("already subscribed")
	}

	if pa.source != nil || pa.hasExternalSource() {
		return fmt.Errorf("someone is already publishing to path '%s'", pa.name)
	}

	pa.addClient(c, clientStatePreRecord)

	pa.source = c
	pa.sourceTrackCount = len(tracks)
	pa.sourceSdp = tracks.Write()
	return nil
}

func (pa *Path) onClientRecord(c *client.Client) {
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

func (pa *Path) onClientPause(c *client.Client) {
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
	if pa.closeTimerStarted ||
		pa.conf.Regexp == nil ||
		pa.hasClients() ||
		pa.source != nil {
		return
	}

	pa.closeTimer.Stop()
	pa.closeTimer = time.NewTimer(0)
	pa.closeTimerStarted = true
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

// SourceTrackCount returns the number of tracks of the source this path.
func (pa *Path) SourceTrackCount() int {
	return pa.sourceTrackCount
}

// OnSourceSetReady is called by a source.
func (pa *Path) OnSourceSetReady(tracks gortsplib.Tracks) {
	pa.sourceSdp = tracks.Write()
	pa.sourceTrackCount = len(tracks)
	pa.sourceSetReady <- struct{}{}
}

// OnSourceSetNotReady is called by a source.
func (pa *Path) OnSourceSetNotReady() {
	pa.sourceSetNotReady <- struct{}{}
}

// OnPathManDescribe is called by pathman.PathMan.
func (pa *Path) OnPathManDescribe(req ClientDescribeReq) {
	pa.clientDescribe <- req
}

// OnPathManSetupPlay is called by pathman.PathMan.
func (pa *Path) OnPathManSetupPlay(req ClientSetupPlayReq) {
	pa.clientSetupPlay <- req
}

// OnPathManAnnounce is called by pathman.PathMan.
func (pa *Path) OnPathManAnnounce(req ClientAnnounceReq) {
	pa.clientAnnounce <- req
}

// OnClientRemove is called by client.Client.
func (pa *Path) OnClientRemove(c *client.Client) {
	res := make(chan struct{})
	pa.clientRemove <- clientRemoveReq{res, c}
	<-res
}

// OnClientPlay is called by client.Client.
func (pa *Path) OnClientPlay(c *client.Client) {
	res := make(chan struct{})
	pa.clientPlay <- clientPlayReq{res, c}
	<-res
}

// OnClientRecord is called by client.Client.
func (pa *Path) OnClientRecord(c *client.Client) {
	res := make(chan struct{})
	pa.clientRecord <- clientRecordReq{res, c}
	<-res
}

// OnClientPause is called by client.Client.
func (pa *Path) OnClientPause(c *client.Client) {
	res := make(chan struct{})
	pa.clientPause <- clientPauseReq{res, c}
	<-res
}

// OnFrame is called by a source or by a client.Client.
func (pa *Path) OnFrame(trackID int, streamType gortsplib.StreamType, buf []byte) {
	pa.readers.forwardFrame(trackID, streamType, buf)
}
