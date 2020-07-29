package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/sdp/v3"
	"gopkg.in/alecthomas/kingpin.v2"
)

var Version = "v0.0.0"

type programEvent interface {
	isProgramEvent()
}

type programEventClientNew struct {
	nconn net.Conn
}

func (programEventClientNew) isProgramEvent() {}

type programEventClientClose struct {
	done   chan struct{}
	client *client
}

func (programEventClientClose) isProgramEvent() {}

type programEventClientDescribe struct {
	client *client
	path   string
}

func (programEventClientDescribe) isProgramEvent() {}

type programEventClientAnnounce struct {
	res       chan error
	client    *client
	path      string
	sdpText   []byte
	sdpParsed *sdp.SessionDescription
}

func (programEventClientAnnounce) isProgramEvent() {}

type programEventClientSetupPlay struct {
	res      chan error
	client   *client
	path     string
	protocol gortsplib.StreamProtocol
	rtpPort  int
	rtcpPort int
}

func (programEventClientSetupPlay) isProgramEvent() {}

type programEventClientSetupRecord struct {
	res      chan error
	client   *client
	protocol gortsplib.StreamProtocol
	rtpPort  int
	rtcpPort int
}

func (programEventClientSetupRecord) isProgramEvent() {}

type programEventClientPlay1 struct {
	res    chan error
	client *client
}

func (programEventClientPlay1) isProgramEvent() {}

type programEventClientPlay2 struct {
	done   chan struct{}
	client *client
}

func (programEventClientPlay2) isProgramEvent() {}

type programEventClientPlayStop struct {
	done   chan struct{}
	client *client
}

func (programEventClientPlayStop) isProgramEvent() {}

type programEventClientRecord struct {
	done   chan struct{}
	client *client
}

func (programEventClientRecord) isProgramEvent() {}

type programEventClientRecordStop struct {
	done   chan struct{}
	client *client
}

func (programEventClientRecordStop) isProgramEvent() {}

type programEventClientFrameUdp struct {
	addr       *net.UDPAddr
	streamType gortsplib.StreamType
	buf        []byte
}

func (programEventClientFrameUdp) isProgramEvent() {}

type programEventClientFrameTcp struct {
	path       string
	trackId    int
	streamType gortsplib.StreamType
	buf        []byte
}

func (programEventClientFrameTcp) isProgramEvent() {}

type programEventSourceReady struct {
	source *source
}

func (programEventSourceReady) isProgramEvent() {}

type programEventSourceNotReady struct {
	source *source
}

func (programEventSourceNotReady) isProgramEvent() {}

type programEventSourceFrame struct {
	source     *source
	trackId    int
	streamType gortsplib.StreamType
	buf        []byte
}

func (programEventSourceFrame) isProgramEvent() {}

type programEventSourceReset struct {
	source *source
}

func (programEventSourceReset) isProgramEvent() {}

type programEventTerminate struct{}

func (programEventTerminate) isProgramEvent() {}

type program struct {
	conf           *conf
	rtspl          *serverTcp
	rtpl           *serverUdp
	rtcpl          *serverUdp
	sources        []*source
	clients        map[*client]struct{}
	paths          map[string]*path
	publisherCount int
	readerCount    int

	events chan programEvent
	done   chan struct{}
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

	p := &program{
		conf:    conf,
		clients: make(map[*client]struct{}),
		paths:   make(map[string]*path),
		events:  make(chan programEvent),
		done:    make(chan struct{}),
	}

	for path, pconf := range conf.Paths {
		if pconf.Source != "record" {
			s := newSource(p, path, pconf)
			p.sources = append(p.sources, s)
			p.paths[path] = newPath(p, path, s)
		}
	}

	p.log("rtsp-simple-server %s", Version)

	if conf.Pprof {
		go func(mux *http.ServeMux) {
			server := &http.Server{
				Addr:    ":9999",
				Handler: mux,
			}
			p.log("pprof is available on :9999")
			panic(server.ListenAndServe())
		}(http.DefaultServeMux)
		http.DefaultServeMux = http.NewServeMux()
	}

	p.rtpl, err = newServerUdp(p, conf.RtpPort, gortsplib.StreamTypeRtp)
	if err != nil {
		return nil, err
	}

	p.rtcpl, err = newServerUdp(p, conf.RtcpPort, gortsplib.StreamTypeRtcp)
	if err != nil {
		return nil, err
	}

	p.rtspl, err = newServerTcp(p)
	if err != nil {
		return nil, err
	}

	go p.rtpl.run()
	go p.rtcpl.run()
	go p.rtspl.run()
	for _, s := range p.sources {
		go s.run()
	}
	go p.run()

	return p, nil
}

func (p *program) log(format string, args ...interface{}) {
	log.Printf("[%d/%d/%d] "+format, append([]interface{}{len(p.clients),
		p.publisherCount, p.readerCount}, args...)...)
}

func (p *program) run() {
	checkPathsTicker := time.NewTicker(5 * time.Second)
	defer checkPathsTicker.Stop()

outer:
	for {
		select {
		case <-checkPathsTicker.C:
			for _, path := range p.paths {
				path.check()
			}

		case rawEvt := <-p.events:
			switch evt := rawEvt.(type) {
			case programEventClientNew:
				c := newServerClient(p, evt.nconn)
				p.clients[c] = struct{}{}
				c.log("connected")

			case programEventClientClose:
				delete(p.clients, evt.client)

				if evt.client.path != "" {
					if path, ok := p.paths[evt.client.path]; ok {
						// if this is a publisher
						if path.publisher == evt.client {
							path.publisherReset()

							// delete the path
							delete(p.paths, evt.client.path)
						}
					}
				}

				evt.client.log("disconnected")
				close(evt.done)

			case programEventClientDescribe:
				path, ok := p.paths[evt.path]

				// no path: return 404
				if !ok {
					evt.client.describeRes <- nil
					continue
				}

				sdpText, wait := path.describe()

				if wait {
					evt.client.path = evt.path
					evt.client.state = clientStateWaitingDescription
					continue
				}

				evt.client.describeRes <- sdpText

			case programEventClientAnnounce:
				_, ok := p.paths[evt.path]
				if ok {
					evt.res <- fmt.Errorf("someone is already publishing on path '%s'", evt.path)
					continue
				}

				evt.client.path = evt.path
				evt.client.state = clientStateAnnounce
				p.paths[evt.path] = newPath(p, evt.path, evt.client)
				p.paths[evt.path].publisherSdpText = evt.sdpText
				p.paths[evt.path].publisherSdpParsed = evt.sdpParsed
				evt.res <- nil

			case programEventClientSetupPlay:
				path, ok := p.paths[evt.path]
				if !ok || !path.publisherReady {
					evt.res <- fmt.Errorf("no one is publishing on path '%s'", evt.path)
					continue
				}

				if len(evt.client.streamTracks) >= len(path.publisherSdpParsed.MediaDescriptions) {
					evt.res <- fmt.Errorf("all the tracks have already been setup")
					continue
				}

				evt.client.path = evt.path
				evt.client.streamProtocol = evt.protocol
				evt.client.streamTracks = append(evt.client.streamTracks, &clientTrack{
					rtpPort:  evt.rtpPort,
					rtcpPort: evt.rtcpPort,
				})
				evt.client.state = clientStatePrePlay
				evt.res <- nil

			case programEventClientSetupRecord:
				evt.client.streamProtocol = evt.protocol
				evt.client.streamTracks = append(evt.client.streamTracks, &clientTrack{
					rtpPort:  evt.rtpPort,
					rtcpPort: evt.rtcpPort,
				})
				evt.client.state = clientStatePreRecord
				evt.res <- nil

			case programEventClientPlay1:
				path, ok := p.paths[evt.client.path]
				if !ok || !path.publisherReady {
					evt.res <- fmt.Errorf("no one is publishing on path '%s'", evt.client.path)
					continue
				}

				if len(evt.client.streamTracks) != len(path.publisherSdpParsed.MediaDescriptions) {
					evt.res <- fmt.Errorf("not all tracks have been setup")
					continue
				}

				evt.res <- nil

			case programEventClientPlay2:
				p.readerCount += 1
				evt.client.state = clientStatePlay
				close(evt.done)

			case programEventClientPlayStop:
				p.readerCount -= 1
				evt.client.state = clientStatePrePlay
				close(evt.done)

			case programEventClientRecord:
				p.publisherCount += 1
				evt.client.state = clientStateRecord
				p.paths[evt.client.path].publisherSetReady()
				close(evt.done)

			case programEventClientRecordStop:
				p.publisherCount -= 1
				evt.client.state = clientStatePreRecord
				p.paths[evt.client.path].publisherSetNotReady()
				close(evt.done)

			case programEventClientFrameUdp:
				client, trackId := p.findClientPublisher(evt.addr, evt.streamType)
				if client == nil {
					continue
				}

				client.rtcpReceivers[trackId].OnFrame(evt.streamType, evt.buf)
				p.forwardFrame(client.path, trackId, evt.streamType, evt.buf)

			case programEventClientFrameTcp:
				p.forwardFrame(evt.path, evt.trackId, evt.streamType, evt.buf)

			case programEventSourceReady:
				evt.source.log("ready")
				p.paths[evt.source.path].publisherSetReady()

			case programEventSourceNotReady:
				evt.source.log("not ready")
				p.paths[evt.source.path].publisherSetNotReady()

			case programEventSourceFrame:
				p.forwardFrame(evt.source.path, evt.trackId, evt.streamType, evt.buf)

			case programEventSourceReset:
				p.paths[evt.source.path].publisherReset()

			case programEventTerminate:
				break outer
			}
		}
	}

	go func() {
		for rawEvt := range p.events {
			switch evt := rawEvt.(type) {
			case programEventClientClose:
				close(evt.done)

			case programEventClientDescribe:
				evt.client.describeRes <- nil

			case programEventClientAnnounce:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientSetupPlay:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientSetupRecord:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientPlay1:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientPlay2:
				close(evt.done)

			case programEventClientPlayStop:
				close(evt.done)

			case programEventClientRecord:
				close(evt.done)

			case programEventClientRecordStop:
				close(evt.done)
			}
		}
	}()

	for _, s := range p.sources {
		s.events <- sourceEventTerminate{}
		<-s.done
	}

	p.rtspl.close()
	p.rtcpl.close()
	p.rtpl.close()

	for c := range p.clients {
		c.conn.NetConn().Close()
		<-c.done
	}

	close(p.events)
	close(p.done)
}

func (p *program) close() {
	p.events <- programEventTerminate{}
	<-p.done
}

func (p *program) findConfForPath(path string) *confPath {
	if pconf, ok := p.conf.Paths[path]; ok {
		return pconf
	}

	if pconf, ok := p.conf.Paths["all"]; ok {
		return pconf
	}

	return nil
}

func (p *program) findClientPublisher(addr *net.UDPAddr, streamType gortsplib.StreamType) (*client, int) {
	for _, path := range p.paths {
		cl, ok := path.publisher.(*client)
		if !ok {
			continue
		}

		if cl.streamProtocol != gortsplib.StreamProtocolUdp ||
			cl.state != clientStateRecord ||
			!cl.ip().Equal(addr.IP) {
			continue
		}

		for i, t := range cl.streamTracks {
			if streamType == gortsplib.StreamTypeRtp {
				if t.rtpPort == addr.Port {
					return cl, i
				}
			} else {
				if t.rtcpPort == addr.Port {
					return cl, i
				}
			}
		}
	}
	return nil, -1
}

func (p *program) forwardFrame(path string, trackId int, streamType gortsplib.StreamType, frame []byte) {
	for client := range p.clients {
		if client.path == path && client.state == clientStatePlay {
			if client.streamProtocol == gortsplib.StreamProtocolUdp {
				if streamType == gortsplib.StreamTypeRtp {
					p.rtpl.write(&udpAddrBufPair{
						addr: &net.UDPAddr{
							IP:   client.ip(),
							Zone: client.zone(),
							Port: client.streamTracks[trackId].rtpPort,
						},
						buf: frame,
					})
				} else {
					p.rtcpl.write(&udpAddrBufPair{
						addr: &net.UDPAddr{
							IP:   client.ip(),
							Zone: client.zone(),
							Port: client.streamTracks[trackId].rtcpPort,
						},
						buf: frame,
					})
				}

			} else {
				buf := client.writeBuf.swap()
				buf = buf[:len(frame)]
				copy(buf, frame)

				client.events <- clientEventFrameTcp{
					frame: &gortsplib.InterleavedFrame{
						TrackId:    trackId,
						StreamType: streamType,
						Content:    buf,
					},
				}
			}
		}
	}
}

func main() {
	_, err := newProgram(os.Args[1:], os.Stdin)
	if err != nil {
		log.Fatal("ERR:", err)
	}

	select {}
}
