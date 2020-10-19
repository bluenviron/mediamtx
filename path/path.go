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
	"github.com/aler9/rtsp-simple-server/serverudp"
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

// a source can be a client, a sourcertsp.Source or a sourcertmp.Source
type source interface {
	IsSource()
}

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
)

type Path struct {
	wg            *sync.WaitGroup
	stats         *stats.Stats
	serverUdpRtp  *serverudp.Server
	serverUdpRtcp *serverudp.Server
	readTimeout   time.Duration
	writeTimeout  time.Duration
	name          string
	conf          *conf.PathConf
	parent        Parent

	clients                map[*client.Client]clientState
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
	wg *sync.WaitGroup,
	stats *stats.Stats,
	serverUdpRtp *serverudp.Server,
	serverUdpRtcp *serverudp.Server,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	name string,
	conf *conf.PathConf,
	parent Parent) *Path {

	pa := &Path{
		wg:                wg,
		stats:             stats,
		serverUdpRtp:      serverUdpRtp,
		serverUdpRtcp:     serverUdpRtcp,
		readTimeout:       readTimeout,
		writeTimeout:      writeTimeout,
		name:              name,
		conf:              conf,
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
		state := sourcertsp.StateStopped
		if !pa.conf.SourceOnDemand {
			state = sourcertsp.StateRunning
		}

		s := sourcertsp.New(
			pa.conf.Source,
			pa.conf.SourceProtocolParsed,
			pa.readTimeout,
			pa.writeTimeout,
			state,
			pa)
		pa.source = s

		atomic.AddInt64(pa.stats.CountSourcesRtsp, +1)
		if !pa.conf.SourceOnDemand {
			atomic.AddInt64(pa.stats.CountSourcesRtspRunning, +1)
		}

	} else if strings.HasPrefix(pa.conf.Source, "rtmp://") {
		state := sourcertmp.StateStopped
		if !pa.conf.SourceOnDemand {
			state = sourcertmp.StateRunning
		}

		s := sourcertmp.New(
			pa.conf.Source,
			state,
			pa)
		pa.source = s

		atomic.AddInt64(pa.stats.CountSourcesRtmp, +1)
		if !pa.conf.SourceOnDemand {
			atomic.AddInt64(pa.stats.CountSourcesRtmpRunning, +1)
		}
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
				pa.parent.OnPathClose(pa)
				<-pa.terminate
				break outer
			}

		case <-pa.sourceSetReady:
			pa.onSourceSetReady()

		case <-pa.sourceSetNotReady:
			pa.onSourceSetNotReady()

		case req := <-pa.clientDescribe:
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
			if _, ok := pa.clients[req.client]; ok {
				pa.onClientPlay(req.client)
			}
			close(req.res)

		case req := <-pa.clientAnnounce:
			err := pa.onClientAnnounce(req.Client, req.Tracks)
			if err != nil {
				req.Res <- ClientAnnounceRes{nil, err}
				continue
			}
			req.Res <- ClientAnnounceRes{pa, nil}

		case req := <-pa.clientRecord:
			if _, ok := pa.clients[req.client]; ok {
				pa.onClientRecord(req.client)
			}
			close(req.res)

		case req := <-pa.clientRemove:
			if _, ok := pa.clients[req.client]; ok {
				pa.onClientRemove(req.client)
			}
			close(req.res)

		case <-pa.terminate:
			break outer
		}
	}

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
				close(req.res)
			}
		}
	}()

	if pa.onInitCmd != nil {
		pa.Log("stopping on init command (closing)")
		pa.onInitCmd.Close()
	}

	if source, ok := pa.source.(*sourcertsp.Source); ok {
		source.Close()

	} else if source, ok := pa.source.(*sourcertmp.Source); ok {
		source.Close()
	}

	if pa.onDemandCmd != nil {
		pa.Log("stopping on demand command (closing)")
		pa.onDemandCmd.Close()
	}

	for c, state := range pa.clients {
		if state == clientStateWaitingDescribe {
			delete(pa.clients, c)
			c.OnPathDescribeData(nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name))
		} else {
			pa.onClientRemove(c)
			pa.parent.OnPathClientClose(c)
		}
	}

	close(pa.sourceSetReady)
	close(pa.sourceSetNotReady)
	close(pa.clientDescribe)
	close(pa.clientAnnounce)
	close(pa.clientSetupPlay)
	close(pa.clientPlay)
	close(pa.clientRecord)
	close(pa.clientRemove)
}

func (pa *Path) hasClients() bool {
	return len(pa.clients) > 0
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
	for c := range pa.clients {
		if c != pa.source {
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
			if state == clientStateWaitingDescribe {
				delete(pa.clients, c)
				c.OnPathDescribeData(nil, fmt.Errorf("publisher of path '%s' has timed out", pa.name))
			}
		}
	}

	// stop on demand rtsp source if needed
	if source, ok := pa.source.(*sourcertsp.Source); ok {
		if pa.conf.SourceOnDemand &&
			source.State() == sourcertsp.StateRunning &&
			!pa.hasClients() &&
			time.Since(pa.lastDescribeReq) >= sourceStopAfterDescribePeriod {
			pa.Log("stopping on demand rtsp source (not requested anymore)")
			atomic.AddInt64(pa.stats.CountSourcesRtspRunning, -1)
			source.SetState(sourcertsp.StateStopped)
		}

		// stop on demand rtmp source if needed
	} else if source, ok := pa.source.(*sourcertmp.Source); ok {
		if pa.conf.SourceOnDemand &&
			source.State() == sourcertmp.StateRunning &&
			!pa.hasClients() &&
			time.Since(pa.lastDescribeReq) >= sourceStopAfterDescribePeriod {
			pa.Log("stopping on demand rtmp source (not requested anymore)")
			atomic.AddInt64(pa.stats.CountSourcesRtmpRunning, -1)
			source.SetState(sourcertmp.StateStopped)
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

	// remove path if is regexp and has no clients
	if pa.conf.Regexp != nil &&
		pa.source == nil &&
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
			delete(pa.clients, c)
			c.OnPathDescribeData(pa.sourceSdp, nil)
		}
	}
}

func (pa *Path) onSourceSetNotReady() {
	pa.sourceReady = false

	// close all clients that are reading or waiting to read
	for c, state := range pa.clients {
		if state != clientStateWaitingDescribe && c != pa.source {
			pa.onClientRemove(c)
			pa.parent.OnPathClientClose(c)
		}
	}
}

func (pa *Path) onClientDescribe(c *client.Client) {
	pa.lastDescribeReq = time.Now()

	// publisher not found
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

			// no on-demand: reply with 404
		} else {
			c.OnPathDescribeData(nil, fmt.Errorf("no one is publishing on path '%s'", pa.name))
		}

		// publisher was found but is not ready: put the client on hold
	} else if !pa.sourceReady {
		// start rtsp source if needed
		if source, ok := pa.source.(*sourcertsp.Source); ok {
			if source.State() == sourcertsp.StateStopped {
				pa.Log("starting on demand rtsp source")
				pa.lastDescribeActivation = time.Now()
				atomic.AddInt64(pa.stats.CountSourcesRtspRunning, +1)
				source.SetState(sourcertsp.StateRunning)
			}

			// start rtmp source if needed
		} else if source, ok := pa.source.(*sourcertmp.Source); ok {
			if source.State() == sourcertmp.StateStopped {
				pa.Log("starting on demand rtmp source")
				pa.lastDescribeActivation = time.Now()
				atomic.AddInt64(pa.stats.CountSourcesRtmpRunning, +1)
				source.SetState(sourcertmp.StateRunning)
			}
		}

		pa.clients[c] = clientStateWaitingDescribe

		// publisher was found and is ready
	} else {
		c.OnPathDescribeData(pa.sourceSdp, nil)
	}
}

func (pa *Path) onClientSetupPlay(c *client.Client, trackId int) error {
	if !pa.sourceReady {
		return fmt.Errorf("no one is publishing on path '%s'", pa.name)
	}

	if trackId >= pa.sourceTrackCount {
		return fmt.Errorf("track %d does not exist", trackId)
	}

	pa.clients[c] = clientStatePrePlay
	return nil
}

func (pa *Path) onClientPlay(c *client.Client) {
	atomic.AddInt64(pa.stats.CountReaders, 1)
	pa.clients[c] = clientStatePlay
	pa.readers.add(c)
}

func (pa *Path) onClientAnnounce(c *client.Client, tracks gortsplib.Tracks) error {
	if pa.source != nil {
		return fmt.Errorf("someone is already publishing on path '%s'", pa.name)
	}

	pa.clients[c] = clientStatePreRecord
	pa.source = c
	pa.sourceTrackCount = len(tracks)
	pa.sourceSdp = tracks.Write()
	return nil
}

func (pa *Path) onClientRecord(c *client.Client) {
	atomic.AddInt64(pa.stats.CountPublishers, 1)
	pa.clients[c] = clientStateRecord
	pa.onSourceSetReady()
}

func (pa *Path) onClientRemove(c *client.Client) {
	state := pa.clients[c]
	delete(pa.clients, c)

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
			if state != clientStateWaitingDescribe && oc != pa.source {
				pa.onClientRemove(oc)
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

func (pa *Path) Name() string {
	return pa.name
}

func (pa *Path) SourceTrackCount() int {
	return pa.sourceTrackCount
}

func (pa *Path) Conf() *conf.PathConf {
	return pa.conf
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
