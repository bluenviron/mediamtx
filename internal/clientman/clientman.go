package clientman

import (
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/base"
	"github.com/aler9/gortsplib/headers"

	"github.com/aler9/rtsp-simple-server/internal/client"
	"github.com/aler9/rtsp-simple-server/internal/pathman"
	"github.com/aler9/rtsp-simple-server/internal/servertcp"
	"github.com/aler9/rtsp-simple-server/internal/serverudp"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

// Parent is implemented by program.
type Parent interface {
	Log(string, ...interface{})
}

// ClientManager is a client.Client manager.
type ClientManager struct {
	rtspPort            int
	readTimeout         time.Duration
	writeTimeout        time.Duration
	runOnConnect        string
	runOnConnectRestart bool
	protocols           map[headers.StreamProtocol]struct{}
	stats               *stats.Stats
	serverUdpRtp        *serverudp.Server
	serverUdpRtcp       *serverudp.Server
	pathMan             *pathman.PathManager
	serverTcp           *servertcp.Server
	parent              Parent

	clients map[*client.Client]struct{}
	wg      sync.WaitGroup

	// in
	clientClose chan *client.Client
	terminate   chan struct{}

	// out
	done chan struct{}
}

// New allocates a ClientManager.
func New(
	rtspPort int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	runOnConnect string,
	runOnConnectRestart bool,
	protocols map[headers.StreamProtocol]struct{},
	stats *stats.Stats,
	serverUdpRtp *serverudp.Server,
	serverUdpRtcp *serverudp.Server,
	pathMan *pathman.PathManager,
	serverTcp *servertcp.Server,
	parent Parent) *ClientManager {

	cm := &ClientManager{
		rtspPort:            rtspPort,
		readTimeout:         readTimeout,
		writeTimeout:        writeTimeout,
		runOnConnect:        runOnConnect,
		runOnConnectRestart: runOnConnectRestart,
		protocols:           protocols,
		stats:               stats,
		serverUdpRtp:        serverUdpRtp,
		serverUdpRtcp:       serverUdpRtcp,
		pathMan:             pathMan,
		serverTcp:           serverTcp,
		parent:              parent,
		clients:             make(map[*client.Client]struct{}),
		clientClose:         make(chan *client.Client),
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
func (cm *ClientManager) Log(format string, args ...interface{}) {
	cm.parent.Log(format, args...)
}

func (cm *ClientManager) run() {
	defer close(cm.done)

outer:
	for {
		select {
		case conn := <-cm.serverTcp.Accept():
			c := client.New(cm.rtspPort, cm.readTimeout, cm.writeTimeout,
				cm.runOnConnect, cm.runOnConnectRestart, cm.protocols, &cm.wg,
				cm.stats, cm.serverUdpRtp, cm.serverUdpRtcp, conn, cm)
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
		for {
			select {
			case <-cm.clientClose:
			}
		}
	}()

	for c := range cm.clients {
		c.Close()
	}
	cm.wg.Wait()

	close(cm.clientClose)
}

// OnClientClose is called by client.Client.
func (cm *ClientManager) OnClientClose(c *client.Client) {
	cm.clientClose <- c
}

// OnClientDescribe is called by client.Client.
func (cm *ClientManager) OnClientDescribe(c *client.Client, pathName string, req *base.Request) (client.Path, error) {
	return cm.pathMan.OnClientDescribe(c, pathName, req)
}

// OnClientAnnounce is called by client.Client.
func (cm *ClientManager) OnClientAnnounce(c *client.Client, pathName string, tracks gortsplib.Tracks, req *base.Request) (client.Path, error) {
	return cm.pathMan.OnClientAnnounce(c, pathName, tracks, req)
}

// OnClientSetupPlay is called by client.Client.
func (cm *ClientManager) OnClientSetupPlay(c *client.Client, pathName string, trackId int, req *base.Request) (client.Path, error) {
	return cm.pathMan.OnClientSetupPlay(c, pathName, trackId, req)
}
