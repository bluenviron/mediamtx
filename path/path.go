package path

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/base"

	"github.com/aler9/rtsp-simple-server/client"
	"github.com/aler9/rtsp-simple-server/conf"
	"github.com/aler9/rtsp-simple-server/externalcmd"
	"github.com/aler9/rtsp-simple-server/sourcertmp"
	"github.com/aler9/rtsp-simple-server/sourcertsp"
	"github.com/aler9/rtsp-simple-server/stats"
)

const (
	pathCheckPeriod                    = 5 * time.Second
	describeTimeout                    = 5 * time.Second
	sourceStopAfterDescribePeriod      = 10 * time.Second
	onDemandCmdStopAfterDescribePeriod = 10 * time.Second
)

type Parent interface {
	Log(string, ...interface{})
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
	Close()
	IsRunning() bool
	SetRunning(bool)
}

type sourceRedirect struct{}

func (*sourceRedirect) IsSource() {}

type ClientDescribeRes struct {
	Path client.Path
	Err  error
}

type ClientDescribeReq struct {
	Res      chan ClientDescribeRes
	Client   *client.Client
	PathName string
	Req      *base.Request
}

type ClientAnnounceRes struct {
	Path client.Path
	Err  error
}

type ClientAnnounceReq struct {
	Res      chan ClientAnnounceRes
	Client   *client.Client
	PathName string
	Tracks   gortsplib.Tracks
	Req      *base.Request
}

type ClientSetupPlayRes struct {
	Path client.Path
	Err  error
}

type ClientSetupPlayReq struct {
	Res      chan ClientSetupPlayRes
	Client   *client.Client
	PathName string
	TrackId  int
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

type clientState int

const (
	clientStateWaitingDescribe clientState = iota
	clientStatePrePlay
	clientStatePlay
	clientStatePreRecord
	clientStateRecord
	clientStatePreRemove
)

type Path struct {
	readTimeout  time.Duration
	writeTimeout time.Duration
	confName     string
	conf         *conf.PathConf
	name         string
	wg           *sync.WaitGroup
	stats        *stats.Stats
	parent       Parent

	clients                map[*client.Client]clientState
	clientsWg              sync.WaitGroup
	source                 source
	sourceReady            bool
	sourceTrackCount       int
	sourceSdp              []byte
	lastDescribeReq        time.Time
	lastDescribeActivation time.Time
	readers                *readersMap
	onInitCmd              *externalcmd.ExternalCmd
	onDemandCmd            *externalcmd.ExternalCmd

	// in
	sourceSetReady    chan struct{}           // from source
	sourceSetNotReady chan struct{}           // from source
	clientDescribe    chan ClientDescribeReq  // from program
	clientAnnounce    chan ClientAnnounceReq  // from program
	clientSetupPlay   chan ClientSetupPlayReq // from program
	clientPlay        chan clientPlayReq      // from client
	clientRecord      chan clientRecordReq    // from client
	clientRemove      chan clientRemoveReq    // from client
	terminate         chan struct{}
}

func New(
	readTimeout time.Duration,
	writeTimeout time.Duration,
	confName string,
	conf *conf.PathConf,
	name string,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Path {

	pa := &Path{
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		confName:          confName,
		conf:              conf,
		name:              name,
		wg:                wg,
		stats:             stats,
		parent:            parent,
		clients:           make(map[*client.Client]clientState),
		readers:           newReadersMap(),
		sourceSetReady:    make(chan struct{}),
		sourceSetNotReady: make(chan struct{}),
		clientDescribe:    make(chan ClientDescribeReq),
		clientAnnounce:    make(chan ClientAnnounceReq),
		clientSetupPlay:   make(chan ClientSetupPlayReq),
		clientPlay:        make(chan clientPlayReq),
		clientRecord:      make(chan clientRecordReq),
		clientRemove:      make(chan clientRemoveReq),
		terminate:         make(chan struct{}),
	}

	pa.wg.Add(1)
	go pa.run()
	return pa
}

func (pa *Path) Close() {
	close(pa.terminate)
}

func (pa *Path) Log(format string, args ...interface{}) {
	pa.parent.Log("[path "+pa.name+"] "+format, args...)
}

func (pa *Path) run() {
	defer pa.wg.Done()

	if strings.HasPrefix(pa.conf.Source, "rtsp://") {
		state := !pa.conf.SourceOnDemand
		if state {
			pa.Log("starting source")
		}

		pa.source = sourcertsp.New(pa.conf.Source, pa.conf.SourceProtocolParsed,
			pa.readTimeout, pa.writeTimeout, state, pa.stats, pa)

	} else if strings.HasPrefix(pa.conf.Source, "rtmp://") {
		state := !pa.conf.SourceOnDemand
		if state {
			pa.Log("starting source")
		}

		pa.source = sourcertmp.New(pa.conf.Source, state, pa.stats, pa)

	} else if pa.conf.Source == "redirect" {
		pa.source = &sourceRedirect{}
	}

	if pa.conf.RunOnInit != "" {
		pa.Log("starting on init command")
		var err error
		pa.onInitCmd, err = externalcmd.New(pa.conf.RunOnInit, pa.name)
		if err != nil {
			pa.Log("ERR: %s", err)
		}
	}

	tickerCheck := time.NewTicker(pathCheckPeriod)
	defer tickerCheck.Stop()

outer:
	for {
		select {
		case <-tickerCheck.C:
			ok := pa.onCheck()
			if !ok {
				pa.exhaustChannels()
				pa.parent.OnPathClose(pa)
				<-pa.terminate
				break outer
			}

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
			err := pa.onClientSetupPlay(req.Client, req.TrackId)
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

		case req := <-pa.clientRemove:
			if _, ok := pa.clients[req.client]; !ok {
				close(req.res)
				continue
			}

			if pa.clients[req.client] != clientStatePreRemove {
				pa.onClientPreRemove(req.client)
			}

			delete(pa.clients, req.client)
			pa.clientsWg.Done()

			close(req.res)

		case <-pa.terminate:
			pa.exhaustChannels()
			break outer
		}
	}

	if pa.onInitCmd != nil {
		pa.Log("stopping on init command (closing)")
		pa.onInitCmd.Close()
	}

	if source, ok := pa.source.(sourceExternal); ok {
		if source.IsRunning() {
			pa.Log("stopping on demand source (closing)")
		}
		source.Close()
	}

	if pa.onDemandCmd != nil {
		pa.Log("stopping on demand command (closing)")
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

func (pa *Path) hasClients() bool {
	for _, state := range pa.clients {
		if state != clientStatePreRemove {
			return true
		}
	}
	return false
}

func (pa *Path) hasClientsWaitingDescribe() bool {
	for _, state := range pa.clients {
		if state == clientStateWaitingDescribe {
			return true
		}
	}
	return false
}

func (pa *Path) hasClientReadersOrWaitingDescribe() bool {
	for c, state := range pa.clients {
		if state != clientStatePreRemove && c != pa.source {
			return true
		}
	}
	return false
}

func (pa *Path) onCheck() bool {
	// reply to DESCRIBE requests if they are in timeout
	if pa.hasClientsWaitingDescribe() &&
		time.Since(pa.lastDescribeActivation) >= describeTimeout {
		for c, state := range pa.clients {
			if state != clientStatePreRemove && state == clientStateWaitingDescribe {
				pa.clients[c] = clientStatePreRemove
				c.OnPathDescribeData(nil, "", fmt.Errorf("publisher of path '%s' has timed out", pa.name))
			}
		}
	}

	// stop on demand source if needed
	if source, ok := pa.source.(sourceExternal); ok {
		if pa.conf.SourceOnDemand &&
			source.IsRunning() &&
			!pa.hasClients() &&
			time.Since(pa.lastDescribeReq) >= sourceStopAfterDescribePeriod {
			pa.Log("stopping on demand source (not requested anymore)")
			source.SetRunning(false)
		}
	}

	// stop on demand command if needed
	if pa.onDemandCmd != nil &&
		!pa.hasClientReadersOrWaitingDescribe() &&
		time.Since(pa.lastDescribeReq) >= onDemandCmdStopAfterDescribePeriod {
		pa.Log("stopping on demand command (not requested anymore)")
		pa.onDemandCmd.Close()
		pa.onDemandCmd = nil
	}

	// remove path if is regexp, has no source, has no on-demand command and has no clients
	if pa.conf.Regexp != nil &&
		pa.source == nil &&
		pa.onDemandCmd == nil &&
		!pa.hasClients() {
		return false
	}

	return true
}

func (pa *Path) onSourceSetReady() {
	pa.sourceReady = true

	// reply to all clients that are waiting for a description
	for c, state := range pa.clients {
		if state == clientStateWaitingDescribe {
			pa.clients[c] = clientStatePreRemove
			c.OnPathDescribeData(pa.sourceSdp, "", nil)
		}
	}
}

func (pa *Path) onSourceSetNotReady() {
	pa.sourceReady = false

	// close all clients that are reading or waiting to read
	for c, state := range pa.clients {
		if state != clientStatePreRemove && state != clientStateWaitingDescribe && c != pa.source {
			pa.onClientPreRemove(c)
			pa.parent.OnPathClientClose(c)
		}
	}
}

func (pa *Path) onClientDescribe(c *client.Client) {
	pa.lastDescribeReq = time.Now()

	// source not found
	if pa.source == nil {
		// on demand command is available: put the client on hold
		if pa.conf.RunOnDemand != "" {
			if pa.onDemandCmd == nil { // start if needed
				pa.Log("starting on demand command")
				pa.lastDescribeActivation = time.Now()
				var err error
				pa.onDemandCmd, err = externalcmd.New(pa.conf.RunOnDemand, pa.name)
				if err != nil {
					pa.Log("ERR: %s", err)
				}
			}

			pa.clients[c] = clientStateWaitingDescribe
			pa.clientsWg.Add(1)

			// no on-demand: reply with 404
		} else {
			pa.clients[c] = clientStatePreRemove
			pa.clientsWg.Add(1)

			c.OnPathDescribeData(nil, "", fmt.Errorf("no one is publishing to path '%s'", pa.name))
		}

		// source found and is redirect
	} else if _, ok := pa.source.(*sourceRedirect); ok {
		pa.clients[c] = clientStatePreRemove
		pa.clientsWg.Add(1)

		c.OnPathDescribeData(nil, pa.conf.SourceRedirect, nil)

		// source was found but is not ready: put the client on hold
	} else if !pa.sourceReady {
		// start source if needed
		if source, ok := pa.source.(sourceExternal); ok {
			if !source.IsRunning() {
				pa.Log("starting on demand source")
				pa.lastDescribeActivation = time.Now()
				source.SetRunning(true)
			}
		}

		pa.clients[c] = clientStateWaitingDescribe
		pa.clientsWg.Add(1)

		// source was found and is ready
	} else {
		pa.clients[c] = clientStatePreRemove
		pa.clientsWg.Add(1)

		c.OnPathDescribeData(pa.sourceSdp, "", nil)
	}
}

func (pa *Path) onClientSetupPlay(c *client.Client, trackId int) error {
	if !pa.sourceReady {
		return fmt.Errorf("no one is publishing to path '%s'", pa.name)
	}

	if trackId >= pa.sourceTrackCount {
		return fmt.Errorf("track %d does not exist", trackId)
	}

	if _, ok := pa.clients[c]; !ok {
		pa.clients[c] = clientStatePrePlay
		pa.clientsWg.Add(1)
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

	if pa.source != nil {
		return fmt.Errorf("someone is already publishing to path '%s'", pa.name)
	}

	pa.clients[c] = clientStatePreRecord
	pa.clientsWg.Add(1)

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

func (pa *Path) onClientPreRemove(c *client.Client) {
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
			if state != clientStatePreRemove && state != clientStateWaitingDescribe && oc != pa.source {
				pa.onClientPreRemove(oc)
				pa.parent.OnPathClientClose(oc)
			}
		}
	}
}

func (pa *Path) OnSourceReady(tracks gortsplib.Tracks) {
	pa.sourceSdp = tracks.Write()
	pa.sourceTrackCount = len(tracks)
	pa.sourceSetReady <- struct{}{}
}

func (pa *Path) OnSourceNotReady() {
	pa.sourceSetNotReady <- struct{}{}
}

func (pa *Path) ConfName() string {
	return pa.confName
}

func (pa *Path) Conf() *conf.PathConf {
	return pa.conf
}

func (pa *Path) Name() string {
	return pa.name
}

func (pa *Path) SourceTrackCount() int {
	return pa.sourceTrackCount
}

func (pa *Path) OnPathManDescribe(req ClientDescribeReq) {
	pa.clientDescribe <- req
}

func (pa *Path) OnPathManSetupPlay(req ClientSetupPlayReq) {
	pa.clientSetupPlay <- req
}

func (pa *Path) OnPathManAnnounce(req ClientAnnounceReq) {
	pa.clientAnnounce <- req
}

func (pa *Path) OnClientRemove(c *client.Client) {
	res := make(chan struct{})
	pa.clientRemove <- clientRemoveReq{res, c}
	<-res
}

func (pa *Path) OnClientPlay(c *client.Client) {
	res := make(chan struct{})
	pa.clientPlay <- clientPlayReq{res, c}
	<-res
}

func (pa *Path) OnClientRecord(c *client.Client) {
	res := make(chan struct{})
	pa.clientRecord <- clientRecordReq{res, c}
	<-res
}

func (pa *Path) OnFrame(trackId int, streamType gortsplib.StreamType, buf []byte) {
	pa.readers.forwardFrame(trackId, streamType, buf)
}
