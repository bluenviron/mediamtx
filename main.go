package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"gopkg.in/alecthomas/kingpin.v2"
)

var Version = "v0.0.0"

const (
	checkPathPeriod = 5 * time.Second
)

type program struct {
	conf             *conf
	logHandler       *logHandler
	metrics          *metrics
	pprof            *pprof
	paths            map[string]*path
	serverRtp        *serverUDP
	serverRtcp       *serverUDP
	serverRtsp       *serverTCP
	clients          map[*client]struct{}
	udpPublishersMap *udpPublishersMap
	readersMap       *readersMap
	countClients     int64
	countPublishers  int64
	countReaders     int64

	clientNew       chan net.Conn
	clientClose     chan *client
	clientDescribe  chan clientDescribeReq
	clientAnnounce  chan clientAnnounceReq
	clientSetupPlay chan clientSetupPlayReq
	clientPlay      chan *client
	clientRecord    chan *client
	sourceReady     chan *source
	sourceNotReady  chan *source
	terminate       chan struct{}
	done            chan struct{}
}

func newProgram(args []string, stdin io.Reader) (*program, error) {
	k := kingpin.New("rtsp-simple-server",
		"rtsp-simple-server "+Version+"\n\nRTSP server.")

	argVersion := k.Flag("version", "print version").Bool()
	argConfPath := k.Arg("confpath", "path to a config file. The default is rtsp-simple-server.yml. Use 'stdin' to read config from stdin").Default("rtsp-simple-server.yml").String()

	kingpin.MustParse(k.Parse(args))

	if *argVersion == true {
		fmt.Println(Version)
		os.Exit(0)
	}

	conf, err := loadConf(*argConfPath, stdin)
	if err != nil {
		return nil, err
	}

	logHandler, err := newLogHandler(conf.logDestinationsParsed, conf.LogFile)
	if err != nil {
		return nil, err
	}

	p := &program{
		conf:             conf,
		logHandler:       logHandler,
		paths:            make(map[string]*path),
		clients:          make(map[*client]struct{}),
		udpPublishersMap: newUdpPublisherMap(),
		readersMap:       newReadersMap(),
		clientNew:        make(chan net.Conn),
		clientClose:      make(chan *client),
		clientDescribe:   make(chan clientDescribeReq),
		clientAnnounce:   make(chan clientAnnounceReq),
		clientSetupPlay:  make(chan clientSetupPlayReq),
		clientPlay:       make(chan *client),
		clientRecord:     make(chan *client),
		sourceReady:      make(chan *source),
		sourceNotReady:   make(chan *source),
		terminate:        make(chan struct{}),
		done:             make(chan struct{}),
	}

	p.log("rtsp-simple-server %s", Version)

	if conf.Metrics {
		p.metrics, err = newMetrics(p)
		if err != nil {
			return nil, err
		}
	}

	if conf.Pprof {
		p.pprof, err = newPprof(p)
		if err != nil {
			return nil, err
		}
	}

	for name, pathConf := range conf.Paths {
		if pathConf.regexp == nil {
			p.paths[name] = newPath(p, name, pathConf)
		}
	}

	if _, ok := conf.protocolsParsed[gortsplib.StreamProtocolUDP]; ok {
		p.serverRtp, err = newServerUDP(p, conf.RtpPort, gortsplib.StreamTypeRtp)
		if err != nil {
			return nil, err
		}

		p.serverRtcp, err = newServerUDP(p, conf.RtcpPort, gortsplib.StreamTypeRtcp)
		if err != nil {
			return nil, err
		}
	}

	p.serverRtsp, err = newServerTCP(p)
	if err != nil {
		return nil, err
	}

	go p.run()

	return p, nil
}

func (p *program) log(format string, args ...interface{}) {
	countClients := atomic.LoadInt64(&p.countClients)
	countPublishers := atomic.LoadInt64(&p.countPublishers)
	countReaders := atomic.LoadInt64(&p.countReaders)

	log.Printf(fmt.Sprintf("[%d/%d/%d] "+format, append([]interface{}{countClients,
		countPublishers, countReaders}, args...)...))
}

func (p *program) run() {
	if p.metrics != nil {
		go p.metrics.run()
	}

	if p.pprof != nil {
		go p.pprof.run()
	}

	if p.serverRtp != nil {
		go p.serverRtp.run()
	}

	if p.serverRtcp != nil {
		go p.serverRtcp.run()
	}

	go p.serverRtsp.run()

	for _, p := range p.paths {
		p.onInit()
	}

	checkPathsTicker := time.NewTicker(checkPathPeriod)
	defer checkPathsTicker.Stop()

outer:
	for {
		select {
		case <-checkPathsTicker.C:
			for _, path := range p.paths {
				path.onCheck()
			}

		case conn := <-p.clientNew:
			c := newClient(p, conn)
			p.clients[c] = struct{}{}
			atomic.AddInt64(&p.countClients, 1)
			c.log("connected")

		case client := <-p.clientClose:
			if _, ok := p.clients[client]; !ok {
				continue
			}
			client.close()

		case req := <-p.clientDescribe:
			// create path if not exist
			if _, ok := p.paths[req.pathName]; !ok {
				p.paths[req.pathName] = newPath(p, req.pathName, req.pathConf)
			}

			p.paths[req.pathName].onDescribe(req.client)

		case req := <-p.clientAnnounce:
			// create path if not exist
			if path, ok := p.paths[req.pathName]; !ok {
				p.paths[req.pathName] = newPath(p, req.pathName, req.pathConf)

			} else {
				if path.publisher != nil {
					req.res <- fmt.Errorf("someone is already publishing on path '%s'", req.pathName)
					continue
				}
			}

			p.paths[req.pathName].publisher = req.client
			p.paths[req.pathName].publisherTrackCount = req.trackCount
			p.paths[req.pathName].publisherSdp = req.sdp

			req.client.path = p.paths[req.pathName]
			req.client.state = clientStatePreRecord
			req.res <- nil

		case req := <-p.clientSetupPlay:
			path, ok := p.paths[req.pathName]
			if !ok || !path.publisherReady {
				req.res <- fmt.Errorf("no one is publishing on path '%s'", req.pathName)
				continue
			}

			if req.trackId >= path.publisherTrackCount {
				req.res <- fmt.Errorf("track %d does not exist", req.trackId)
				continue
			}

			req.client.path = path
			req.client.state = clientStatePrePlay
			req.res <- nil

		case client := <-p.clientPlay:
			atomic.AddInt64(&p.countReaders, 1)
			client.state = clientStatePlay
			p.readersMap.add(client)

		case client := <-p.clientRecord:
			atomic.AddInt64(&p.countPublishers, 1)
			client.state = clientStateRecord

			if client.streamProtocol == gortsplib.StreamProtocolUDP {
				for trackId, track := range client.streamTracks {
					addr := makeUDPPublisherAddr(client.ip(), track.rtpPort)
					p.udpPublishersMap.add(addr, &udpPublisher{
						client:     client,
						trackId:    trackId,
						streamType: gortsplib.StreamTypeRtp,
					})

					addr = makeUDPPublisherAddr(client.ip(), track.rtcpPort)
					p.udpPublishersMap.add(addr, &udpPublisher{
						client:     client,
						trackId:    trackId,
						streamType: gortsplib.StreamTypeRtcp,
					})
				}
			}

			client.path.onPublisherSetReady()

		case source := <-p.sourceReady:
			source.path.log("source ready")
			source.path.onPublisherSetReady()

		case source := <-p.sourceNotReady:
			source.path.log("source not ready")
			source.path.onPublisherSetNotReady()

		case <-p.terminate:
			break outer
		}
	}

	go func() {
		for {
			select {
			case _, ok := <-p.clientNew:
				if !ok {
					return
				}

			case <-p.clientClose:
			case <-p.clientDescribe:

			case req := <-p.clientAnnounce:
				req.res <- fmt.Errorf("terminated")

			case req := <-p.clientSetupPlay:
				req.res <- fmt.Errorf("terminated")

			case <-p.clientPlay:
			case <-p.clientRecord:
			case <-p.sourceReady:
			case <-p.sourceNotReady:
			}
		}
	}()

	p.udpPublishersMap.clear()
	p.readersMap.clear()

	for _, p := range p.paths {
		p.onClose(true)
	}

	p.serverRtsp.close()

	if p.serverRtcp != nil {
		p.serverRtcp.close()
	}

	if p.serverRtp != nil {
		p.serverRtp.close()
	}

	for c := range p.clients {
		c.close()
		<-c.done
	}

	if p.metrics != nil {
		p.metrics.close()
	}

	if p.pprof != nil {
		p.pprof.close()
	}

	p.logHandler.close()

	close(p.clientNew)
	close(p.clientClose)
	close(p.clientDescribe)
	close(p.clientAnnounce)
	close(p.clientSetupPlay)
	close(p.clientPlay)
	close(p.clientRecord)
	close(p.sourceReady)
	close(p.sourceNotReady)
	close(p.done)
}

func (p *program) close() {
	close(p.terminate)
	<-p.done
}

func main() {
	_, err := newProgram(os.Args[1:], os.Stdin)
	if err != nil {
		log.Fatal("ERR: ", err)
	}

	select {}
}
