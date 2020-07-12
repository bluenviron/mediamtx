package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/aler9/gortsplib"
	"github.com/pion/sdp"
	"gopkg.in/alecthomas/kingpin.v2"
)

var Version = "v0.0.0"

type track struct {
	rtpPort  int
	rtcpPort int
}

type streamProtocol int

const (
	_STREAM_PROTOCOL_UDP streamProtocol = iota
	_STREAM_PROTOCOL_TCP
)

func (s streamProtocol) String() string {
	if s == _STREAM_PROTOCOL_UDP {
		return "udp"
	}
	return "tcp"
}

type programEvent interface {
	isProgramEvent()
}

type programEventClientNew struct {
	nconn net.Conn
}

func (programEventClientNew) isProgramEvent() {}

type programEventClientClose struct {
	done   chan struct{}
	client *serverClient
}

func (programEventClientClose) isProgramEvent() {}

type programEventClientDescribe struct {
	path string
	res  chan []byte
}

func (programEventClientDescribe) isProgramEvent() {}

type programEventClientAnnounce struct {
	res    chan error
	client *serverClient
	path   string
}

func (programEventClientAnnounce) isProgramEvent() {}

type programEventClientSetupPlay struct {
	res      chan error
	client   *serverClient
	path     string
	protocol streamProtocol
	rtpPort  int
	rtcpPort int
}

func (programEventClientSetupPlay) isProgramEvent() {}

type programEventClientSetupRecord struct {
	res      chan error
	client   *serverClient
	protocol streamProtocol
	rtpPort  int
	rtcpPort int
}

func (programEventClientSetupRecord) isProgramEvent() {}

type programEventClientPlay1 struct {
	res    chan error
	client *serverClient
}

func (programEventClientPlay1) isProgramEvent() {}

type programEventClientPlay2 struct {
	res    chan error
	client *serverClient
}

func (programEventClientPlay2) isProgramEvent() {}

type programEventClientRecord struct {
	res    chan error
	client *serverClient
}

func (programEventClientRecord) isProgramEvent() {}

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

type programEventStreamerReady struct {
	streamer *streamer
}

func (programEventStreamerReady) isProgramEvent() {}

type programEventStreamerNotReady struct {
	streamer *streamer
}

func (programEventStreamerNotReady) isProgramEvent() {}

type programEventStreamerFrame struct {
	streamer   *streamer
	trackId    int
	streamType gortsplib.StreamType
	buf        []byte
}

func (programEventStreamerFrame) isProgramEvent() {}

type programEventTerminate struct{}

func (programEventTerminate) isProgramEvent() {}

// a publisher can be either a serverClient or a streamer
type publisher interface {
	publisherIsReady() bool
	publisherSdpText() []byte
	publisherSdpParsed() *sdp.SessionDescription
}

type program struct {
	conf           *conf
	rtspl          *serverTcpListener
	rtpl           *serverUdpListener
	rtcpl          *serverUdpListener
	clients        map[*serverClient]struct{}
	streamers      []*streamer
	publishers     map[string]publisher
	publisherCount int
	receiverCount  int

	events chan programEvent
	done   chan struct{}
}

func newProgram(sargs []string, stdin io.Reader) (*program, error) {
	k := kingpin.New("rtsp-simple-server",
		"rtsp-simple-server "+Version+"\n\nRTSP server.")

	argVersion := k.Flag("version", "print version").Bool()
	argConfPath := k.Arg("confpath", "path to a config file. The default is conf.yml. Use 'stdin' to read config from stdin").Default("conf.yml").String()

	kingpin.MustParse(k.Parse(sargs))

	if *argVersion == true {
		fmt.Println(Version)
		os.Exit(0)
	}

	conf, err := loadConf(*argConfPath, stdin)
	if err != nil {
		return nil, err
	}

	p := &program{
		conf:       conf,
		clients:    make(map[*serverClient]struct{}),
		publishers: make(map[string]publisher),
		events:     make(chan programEvent),
		done:       make(chan struct{}),
	}

	for path, pconf := range conf.Paths {
		if pconf.Source != "record" {
			s, err := newStreamer(p, path, pconf.Source, pconf.SourceProtocol)
			if err != nil {
				return nil, err
			}

			p.streamers = append(p.streamers, s)
			p.publishers[path] = s
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

	p.rtpl, err = newServerUdpListener(p, conf.RtpPort, gortsplib.StreamTypeRtp)
	if err != nil {
		return nil, err
	}

	p.rtcpl, err = newServerUdpListener(p, conf.RtcpPort, gortsplib.StreamTypeRtcp)
	if err != nil {
		return nil, err
	}

	p.rtspl, err = newServerTcpListener(p)
	if err != nil {
		return nil, err
	}

	go p.rtpl.run()
	go p.rtcpl.run()
	go p.rtspl.run()
	for _, s := range p.streamers {
		go s.run()
	}
	go p.run()

	return p, nil
}

func (p *program) log(format string, args ...interface{}) {
	log.Printf("[%d/%d/%d] "+format, append([]interface{}{len(p.clients),
		p.publisherCount, p.receiverCount}, args...)...)
}

func (p *program) run() {
outer:
	for rawEvt := range p.events {
		switch evt := rawEvt.(type) {
		case programEventClientNew:
			c := newServerClient(p, evt.nconn)
			p.clients[c] = struct{}{}
			c.log("connected")

		case programEventClientClose:
			// already deleted
			if _, ok := p.clients[evt.client]; !ok {
				close(evt.done)
				continue
			}

			delete(p.clients, evt.client)

			if evt.client.path != "" {
				if pub, ok := p.publishers[evt.client.path]; ok && pub == evt.client {
					delete(p.publishers, evt.client.path)

					// if the publisher has disconnected and was ready
					// close all other clients that share the same path
					if pub.publisherIsReady() {
						for oc := range p.clients {
							if oc != evt.client && oc.path == evt.client.path {
								go oc.close()
							}
						}
					}
				}
			}

			switch evt.client.state {
			case _CLIENT_STATE_PLAY:
				p.receiverCount -= 1

			case _CLIENT_STATE_RECORD:
				p.publisherCount -= 1
			}

			evt.client.log("disconnected")
			close(evt.done)

		case programEventClientDescribe:
			pub, ok := p.publishers[evt.path]
			if !ok || !pub.publisherIsReady() {
				evt.res <- nil
				continue
			}

			evt.res <- pub.publisherSdpText()

		case programEventClientAnnounce:
			_, ok := p.publishers[evt.path]
			if ok {
				evt.res <- fmt.Errorf("someone is already publishing on path '%s'", evt.path)
				continue
			}

			evt.client.path = evt.path
			evt.client.state = _CLIENT_STATE_ANNOUNCE
			p.publishers[evt.path] = evt.client
			evt.res <- nil

		case programEventClientSetupPlay:
			pub, ok := p.publishers[evt.path]
			if !ok || !pub.publisherIsReady() {
				evt.res <- fmt.Errorf("no one is streaming on path '%s'", evt.path)
				continue
			}

			sdpParsed := pub.publisherSdpParsed()

			if len(evt.client.streamTracks) >= len(sdpParsed.MediaDescriptions) {
				evt.res <- fmt.Errorf("all the tracks have already been setup")
				continue
			}

			evt.client.path = evt.path
			evt.client.streamProtocol = evt.protocol
			evt.client.streamTracks = append(evt.client.streamTracks, &track{
				rtpPort:  evt.rtpPort,
				rtcpPort: evt.rtcpPort,
			})
			evt.client.state = _CLIENT_STATE_PRE_PLAY
			evt.res <- nil

		case programEventClientSetupRecord:
			evt.client.streamProtocol = evt.protocol
			evt.client.streamTracks = append(evt.client.streamTracks, &track{
				rtpPort:  evt.rtpPort,
				rtcpPort: evt.rtcpPort,
			})
			evt.client.state = _CLIENT_STATE_PRE_RECORD
			evt.res <- nil

		case programEventClientPlay1:
			pub, ok := p.publishers[evt.client.path]
			if !ok || !pub.publisherIsReady() {
				evt.res <- fmt.Errorf("no one is streaming on path '%s'", evt.client.path)
				continue
			}

			sdpParsed := pub.publisherSdpParsed()

			if len(evt.client.streamTracks) != len(sdpParsed.MediaDescriptions) {
				evt.res <- fmt.Errorf("not all tracks have been setup")
				continue
			}

			evt.res <- nil

		case programEventClientPlay2:
			p.receiverCount += 1
			evt.client.state = _CLIENT_STATE_PLAY
			evt.res <- nil

		case programEventClientRecord:
			p.publisherCount += 1
			evt.client.state = _CLIENT_STATE_RECORD
			evt.res <- nil

		case programEventClientFrameUdp:
			client, trackId := p.findPublisher(evt.addr, evt.streamType)
			if client == nil {
				continue
			}

			client.rtcpReceivers[trackId].onFrame(evt.streamType, evt.buf)
			p.forwardFrame(client.path, trackId, evt.streamType, evt.buf)

		case programEventClientFrameTcp:
			p.forwardFrame(evt.path, evt.trackId, evt.streamType, evt.buf)

		case programEventStreamerReady:
			evt.streamer.ready = true
			p.publisherCount += 1
			evt.streamer.log("ready")

		case programEventStreamerNotReady:
			evt.streamer.ready = false
			p.publisherCount -= 1
			evt.streamer.log("not ready")

			// close all clients that share the same path
			for oc := range p.clients {
				if oc.path == evt.streamer.path {
					go oc.close()
				}
			}

		case programEventStreamerFrame:
			p.forwardFrame(evt.streamer.path, evt.trackId, evt.streamType, evt.buf)

		case programEventTerminate:
			break outer
		}
	}

	go func() {
		for rawEvt := range p.events {
			switch evt := rawEvt.(type) {
			case programEventClientClose:
				close(evt.done)

			case programEventClientDescribe:
				evt.res <- nil

			case programEventClientAnnounce:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientSetupPlay:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientSetupRecord:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientPlay1:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientPlay2:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientRecord:
				evt.res <- fmt.Errorf("terminated")
			}
		}
	}()

	for _, s := range p.streamers {
		s.close()
	}

	p.rtspl.close()
	p.rtcpl.close()
	p.rtpl.close()

	for c := range p.clients {
		c.close()
	}

	close(p.events)
	close(p.done)
}

func (p *program) close() {
	p.events <- programEventTerminate{}
	<-p.done
}

func (p *program) findPublisher(addr *net.UDPAddr, streamType gortsplib.StreamType) (*serverClient, int) {
	for _, pub := range p.publishers {
		cl, ok := pub.(*serverClient)
		if !ok {
			continue
		}

		if cl.streamProtocol != _STREAM_PROTOCOL_UDP ||
			cl.state != _CLIENT_STATE_RECORD ||
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
		if client.path == path && client.state == _CLIENT_STATE_PLAY {
			if client.streamProtocol == _STREAM_PROTOCOL_UDP {
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
				channel := gortsplib.ConvTrackIdAndStreamTypeToChannel(trackId, streamType)

				buf := client.writeBuf.swap()
				buf = buf[:len(frame)]
				copy(buf, frame)

				client.events <- serverClientEventFrameTcp{
					frame: &gortsplib.InterleavedFrame{
						Channel: channel,
						Content: buf,
					},
				}
			}
		}
	}
}

func main() {
	_, err := newProgram(os.Args[1:], os.Stdin)
	if err != nil {
		log.Fatal("ERR: ", err)
	}

	select {}
}
