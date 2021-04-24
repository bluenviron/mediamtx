package clientman

import (
	"net"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/clienthls"
	"github.com/aler9/rtsp-simple-server/internal/clientrtmp"
	"github.com/aler9/rtsp-simple-server/internal/clientrtsp"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/serverhls"
	"github.com/aler9/rtsp-simple-server/internal/serverrtmp"
	"github.com/aler9/rtsp-simple-server/internal/serverrtsp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// PathManager is implemented by pathman.PathManager.
type PathManager interface {
	ClientClose() chan client.Client
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
	serverPlain         *serverrtsp.Server
	serverTLS           *serverrtsp.Server
	serverRTMP          *serverrtmp.Server
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
	serverPlain *serverrtsp.Server,
	serverTLS *serverrtsp.Server,
	serverRTMP *serverrtmp.Server,
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
		serverPlain:         serverPlain,
		serverTLS:           serverTLS,
		serverRTMP:          serverRTMP,
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

	tcpAccept := func() chan *gortsplib.ServerConn {
		if cm.serverPlain != nil {
			return cm.serverPlain.Accept()
		}
		return make(chan *gortsplib.ServerConn)
	}()

	tlsAccept := func() chan *gortsplib.ServerConn {
		if cm.serverTLS != nil {
			return cm.serverTLS.Accept()
		}
		return make(chan *gortsplib.ServerConn)
	}()

	rtmpAccept := func() chan net.Conn {
		if cm.serverRTMP != nil {
			return cm.serverRTMP.Accept()
		}
		return make(chan net.Conn)
	}()

	hlsRequest := func() chan serverhls.Request {
		if cm.serverHLS != nil {
			return cm.serverHLS.Request()
		}
		return make(chan serverhls.Request)
	}()

outer:
	for {
		select {
		case conn := <-tcpAccept:
			c := clientrtsp.New(
				false,
				cm.rtspAddress,
				cm.readTimeout,
				cm.runOnConnect,
				cm.runOnConnectRestart,
				cm.protocols,
				&cm.wg,
				cm.stats,
				conn,
				cm.pathMan,
				cm)
			cm.clients[c] = struct{}{}

		case conn := <-tlsAccept:
			c := clientrtsp.New(
				true,
				cm.rtspAddress,
				cm.readTimeout,
				cm.runOnConnect,
				cm.runOnConnectRestart,
				cm.protocols,
				&cm.wg,
				cm.stats,
				conn,
				cm.pathMan,
				cm)
			cm.clients[c] = struct{}{}

		case nconn := <-rtmpAccept:
			c := clientrtmp.New(
				cm.rtspAddress,
				cm.readTimeout,
				cm.writeTimeout,
				cm.readBufferCount,
				cm.runOnConnect,
				cm.runOnConnectRestart,
				&cm.wg,
				cm.stats,
				nconn,
				cm.pathMan,
				cm)
			cm.clients[c] = struct{}{}

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

		case c := <-cm.pathMan.ClientClose():
			if _, ok := cm.clients[c]; !ok {
				continue
			}
			cm.onClientClose(c)

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
		for {
			select {
			case _, ok := <-cm.clientClose:
				if !ok {
					return
				}

			case <-cm.pathMan.ClientClose():
			}
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
