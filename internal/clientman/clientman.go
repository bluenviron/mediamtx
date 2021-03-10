package clientman

import (
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/clientrtmp"
	"github.com/aler9/rtsp-simple-server/internal/clientrtsp"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmputils"
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

// ClientManager is a clientrtsp.Client manager.
type ClientManager struct {
	rtspPort            int
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
	parent              Parent

	clients map[client.Client]struct{}
	wg      sync.WaitGroup

	// in
	clientClose chan client.Client
	terminate   chan struct{}

	// out
	done chan struct{}
}

// New allocates a ClientManager.
func New(
	rtspPort int,
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
	parent Parent) *ClientManager {

	cm := &ClientManager{
		rtspPort:            rtspPort,
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
		parent:              parent,
		clients:             make(map[client.Client]struct{}),
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

	rtmpAccept := func() chan *rtmputils.Conn {
		if cm.serverRTMP != nil {
			return cm.serverRTMP.Accept()
		}
		return make(chan *rtmputils.Conn)
	}()

outer:
	for {
		select {
		case conn := <-tcpAccept:
			c := clientrtsp.New(
				false,
				cm.rtspPort,
				cm.readTimeout,
				cm.runOnConnect,
				cm.runOnConnectRestart,
				cm.protocols,
				&cm.wg,
				cm.stats,
				conn,
				cm)
			cm.clients[c] = struct{}{}

		case conn := <-tlsAccept:
			c := clientrtsp.New(
				true,
				cm.rtspPort,
				cm.readTimeout,
				cm.runOnConnect,
				cm.runOnConnectRestart,
				cm.protocols,
				&cm.wg,
				cm.stats,
				conn,
				cm)
			cm.clients[c] = struct{}{}

		case conn := <-rtmpAccept:
			c := clientrtmp.New(
				cm.rtspPort,
				cm.readTimeout,
				cm.writeTimeout,
				cm.readBufferCount,
				cm.runOnConnect,
				cm.runOnConnectRestart,
				&cm.wg,
				cm.stats,
				conn,
				cm)
			cm.clients[c] = struct{}{}

		case c := <-cm.pathMan.ClientClose():
			if _, ok := cm.clients[c]; !ok {
				continue
			}
			delete(cm.clients, c)
			c.Close()

		case c := <-cm.clientClose:
			if _, ok := cm.clients[c]; !ok {
				continue
			}
			delete(cm.clients, c)
			c.Close()

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

// OnClientClose is called by clientrtsp.Client.
func (cm *ClientManager) OnClientClose(c client.Client) {
	cm.clientClose <- c
}

// OnClientDescribe is called by clientrtsp.Client.
func (cm *ClientManager) OnClientDescribe(req client.DescribeReq) {
	cm.pathMan.OnClientDescribe(req)
}

// OnClientAnnounce is called by clientrtsp.Client.
func (cm *ClientManager) OnClientAnnounce(req client.AnnounceReq) {
	cm.pathMan.OnClientAnnounce(req)
}

// OnClientSetupPlay is called by clientrtsp.Client.
func (cm *ClientManager) OnClientSetupPlay(req client.SetupPlayReq) {
	cm.pathMan.OnClientSetupPlay(req)
}
