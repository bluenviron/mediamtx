package clientman

import (
	"sync"
	"time"

	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/clienthls"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/serverhls"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// PathManager is implemented by pathman.PathManager.
type PathManager interface {
	OnClientDescribe(client.DescribeReq)
	OnClientAnnounce(client.AnnounceReq)
	OnClientSetupPlay(client.SetupPlayReq)
}

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// ClientManager is a client manager.
type ClientManager struct {
	hlsSegmentCount     int
	hlsSegmentDuration  time.Duration
	rtspAddress         string
	readTimeout         time.Duration
	writeTimeout        time.Duration
	readBufferCount     int
	runOnConnect        string
	runOnConnectRestart bool
	protocols           map[base.StreamProtocol]struct{}
	stats               *stats.Stats
	pathMan             PathManager
	serverHLS           *serverhls.Server
	parent              Parent

	clients          map[client.Client]struct{}
	clientsByHLSPath map[string]*clienthls.Client
	wg               sync.WaitGroup

	// in
	clientClose chan client.Client
	terminate   chan struct{}

	// out
	done chan struct{}
}

// New allocates a ClientManager.
func New(
	hlsSegmentCount int,
	hlsSegmentDuration time.Duration,
	rtspAddress string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	runOnConnect string,
	runOnConnectRestart bool,
	protocols map[base.StreamProtocol]struct{},
	stats *stats.Stats,
	pathMan PathManager,
	serverHLS *serverhls.Server,
	parent Parent) *ClientManager {

	cm := &ClientManager{
		hlsSegmentCount:     hlsSegmentCount,
		hlsSegmentDuration:  hlsSegmentDuration,
		rtspAddress:         rtspAddress,
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		readBufferCount:     readBufferCount,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		protocols:           protocols,
		stats:               stats,
		pathMan:             pathMan,
		serverHLS:           serverHLS,
		parent:              parent,
		clients:             make(map[client.Client]struct{}),
		clientsByHLSPath:    make(map[string]*clienthls.Client),
		clientClose:         make(chan client.Client),
		terminate:           make(chan struct{}),
		done:                make(chan struct{}),
	}

	go cm.run()

	return cm
}

// Close closes a ClientManager.
func (cm *ClientManager) Close() {
	close(cm.terminate)
	<-cm.done
}

// Log is the main logging function.
func (cm *ClientManager) Log(level logger.Level, format string, args ...interface{}) {
	cm.parent.Log(level, format, args...)
}

func (cm *ClientManager) run() {
	defer close(cm.done)

	hlsRequest := func() chan serverhls.Request {
		if cm.serverHLS != nil {
			return cm.serverHLS.Request()
		}
		return make(chan serverhls.Request)
	}()

outer:
	for {
		select {
		case req := <-hlsRequest:
			c, ok := cm.clientsByHLSPath[req.Path]
			if !ok {
				c = clienthls.New(
					cm.hlsSegmentCount,
					cm.hlsSegmentDuration,
					cm.readBufferCount,
					&cm.wg,
					cm.stats,
					req.Path,
					cm.pathMan,
					cm)
				cm.clients[c] = struct{}{}
				cm.clientsByHLSPath[req.Path] = c
			}
			c.OnRequest(req)

		case c := <-cm.clientClose:
			if _, ok := cm.clients[c]; !ok {
				continue
			}
			cm.onClientClose(c)

		case <-cm.terminate:
			break outer
		}
	}

	go func() {
		for range cm.clientClose {
		}
	}()

	for c := range cm.clients {
		c.Close()
	}

	cm.wg.Wait()

	close(cm.clientClose)
}

func (cm *ClientManager) onClientClose(c client.Client) {
	delete(cm.clients, c)
	if hc, ok := c.(*clienthls.Client); ok {
		delete(cm.clientsByHLSPath, hc.PathName())
	}
	c.Close()
}

// OnClientClose is called by a client.
func (cm *ClientManager) OnClientClose(c client.Client) {
	cm.clientClose <- c
}
