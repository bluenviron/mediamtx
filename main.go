package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"gopkg.in/alecthomas/kingpin.v2"
	"gortc.io/sdp"
)

var Version = "v0.0.0"

func parseIpCidrList(in string) ([]interface{}, error) {
	if in == "" {
		return nil, nil
	}

	var ret []interface{}
	for _, t := range strings.Split(in, ",") {
		_, ipnet, err := net.ParseCIDR(t)
		if err == nil {
			ret = append(ret, ipnet)
			continue
		}

		ip := net.ParseIP(t)
		if ip != nil {
			ret = append(ret, ip)
			continue
		}

		return nil, fmt.Errorf("unable to parse ip/network '%s'", t)
	}
	return ret, nil
}

type trackFlowType int

const (
	_TRACK_FLOW_RTP trackFlowType = iota
	_TRACK_FLOW_RTCP
)

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

type programEventClientGetStreamSdp struct {
	path string
	res  chan []byte
}

func (programEventClientGetStreamSdp) isProgramEvent() {}

type programEventClientAnnounce struct {
	res       chan error
	client    *serverClient
	path      string
	sdpText   []byte
	sdpParsed *sdp.Message
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

type programEventClientPause struct {
	res    chan error
	client *serverClient
}

func (programEventClientPause) isProgramEvent() {}

type programEventClientRecord struct {
	res    chan error
	client *serverClient
}

func (programEventClientRecord) isProgramEvent() {}

type programEventFrameUdp struct {
	trackFlowType trackFlowType
	addr          *net.UDPAddr
	buf           []byte
}

func (programEventFrameUdp) isProgramEvent() {}

type programEventFrameTcp struct {
	path          string
	trackId       int
	trackFlowType trackFlowType
	buf           []byte
}

func (programEventFrameTcp) isProgramEvent() {}

type programEventTerminate struct{}

func (programEventTerminate) isProgramEvent() {}

type args struct {
	version      bool
	protocolsStr string
	rtspPort     int
	rtpPort      int
	rtcpPort     int
	readTimeout  time.Duration
	writeTimeout time.Duration
	publishUser  string
	publishPass  string
	publishIps   string
	readUser     string
	readPass     string
	readIps      string
	preScript    string
	postScript   string
}

type program struct {
	args           args
	protocols      map[streamProtocol]struct{}
	publishIps     []interface{}
	readIps        []interface{}
	tcpl           *serverTcpListener
	udplRtp        *serverUdpListener
	udplRtcp       *serverUdpListener
	clients        map[*serverClient]struct{}
	publishers     map[string]*serverClient
	publisherCount int
	receiverCount  int

	events chan programEvent
	done   chan struct{}
}

func newProgram(sargs []string) (*program, error) {
	kingpin.CommandLine.Help = "rtsp-simple-server " + Version + "\n\n" +
		"RTSP server."

	argVersion := kingpin.Flag("version", "print version").Bool()
	argProtocolsStr := kingpin.Flag("protocols", "supported protocols").Default("udp,tcp").String()
	argRtspPort := kingpin.Flag("rtsp-port", "port of the RTSP TCP listener").Default("8554").Int()
	argRtpPort := kingpin.Flag("rtp-port", "port of the RTP UDP listener").Default("8000").Int()
	argRtcpPort := kingpin.Flag("rtcp-port", "port of the RTCP UDP listener").Default("8001").Int()
	argReadTimeout := kingpin.Flag("read-timeout", "timeout of read operations").Default("5s").Duration()
	argWriteTimeout := kingpin.Flag("write-timeout", "timeout of write operations").Default("5s").Duration()
	argPublishUser := kingpin.Flag("publish-user", "optional username required to publish").Default("").String()
	argPublishPass := kingpin.Flag("publish-pass", "optional password required to publish").Default("").String()
	argPublishIps := kingpin.Flag("publish-ips", "comma-separated list of IPs or networks (x.x.x.x/24) that can publish").Default("").String()
	argReadUser := kingpin.Flag("read-user", "optional username required to read").Default("").String()
	argReadPass := kingpin.Flag("read-pass", "optional password required to read").Default("").String()
	argReadIps := kingpin.Flag("read-ips", "comma-separated list of IPs or networks (x.x.x.x/24) that can read").Default("").String()
	argPreScript := kingpin.Flag("pre-script", "optional script to run on client connect").Default("").String()
	argPostScript := kingpin.Flag("post-script", "optional script to run on client disconnect").Default("").String()

	kingpin.MustParse(kingpin.CommandLine.Parse(sargs))

	args := args{
		version:      *argVersion,
		protocolsStr: *argProtocolsStr,
		rtspPort:     *argRtspPort,
		rtpPort:      *argRtpPort,
		rtcpPort:     *argRtcpPort,
		readTimeout:  *argReadTimeout,
		writeTimeout: *argWriteTimeout,
		publishUser:  *argPublishUser,
		publishPass:  *argPublishPass,
		publishIps:   *argPublishIps,
		readUser:     *argReadUser,
		readPass:     *argReadPass,
		readIps:      *argReadIps,
		preScript:    *argPreScript,
		postScript:   *argPostScript,
	}

	if args.version == true {
		fmt.Println(Version)
		os.Exit(0)
	}

	protocols := make(map[streamProtocol]struct{})
	for _, proto := range strings.Split(args.protocolsStr, ",") {
		switch proto {
		case "udp":
			protocols[_STREAM_PROTOCOL_UDP] = struct{}{}

		case "tcp":
			protocols[_STREAM_PROTOCOL_TCP] = struct{}{}

		default:
			return nil, fmt.Errorf("unsupported protocol: %s", proto)
		}
	}
	if len(protocols) == 0 {
		return nil, fmt.Errorf("no protocols provided")
	}

	if (args.rtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}
	if args.rtcpPort != (args.rtpPort + 1) {
		return nil, fmt.Errorf("rtcp and rtp ports must be consecutive")
	}

	if args.publishUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.publishUser) {
			return nil, fmt.Errorf("publish username must be alphanumeric")
		}
	}
	if args.publishPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.publishPass) {
			return nil, fmt.Errorf("publish password must be alphanumeric")
		}
	}
	publishIps, err := parseIpCidrList(args.publishIps)
	if err != nil {
		return nil, err
	}

	if args.readUser != "" && args.readPass == "" || args.readUser == "" && args.readPass != "" {
		return nil, fmt.Errorf("read username and password must be both filled")
	}
	if args.readUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.readUser) {
			return nil, fmt.Errorf("read username must be alphanumeric")
		}
	}
	if args.readPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(args.readPass) {
			return nil, fmt.Errorf("read password must be alphanumeric")
		}
	}
	if args.readUser != "" && args.readPass == "" || args.readUser == "" && args.readPass != "" {
		return nil, fmt.Errorf("read username and password must be both filled")
	}
	readIps, err := parseIpCidrList(args.readIps)
	if err != nil {
		return nil, err
	}

	p := &program{
		args:       args,
		protocols:  protocols,
		publishIps: publishIps,
		readIps:    readIps,
		clients:    make(map[*serverClient]struct{}),
		publishers: make(map[string]*serverClient),
		events:     make(chan programEvent),
		done:       make(chan struct{}),
	}

	p.log("rtsp-simple-server %s", Version)

	p.udplRtp, err = newServerUdpListener(p, args.rtpPort, _TRACK_FLOW_RTP)
	if err != nil {
		return nil, err
	}

	p.udplRtcp, err = newServerUdpListener(p, args.rtcpPort, _TRACK_FLOW_RTCP)
	if err != nil {
		return nil, err
	}

	p.tcpl, err = newServerTcpListener(p)
	if err != nil {
		return nil, err
	}

	go p.udplRtp.run()
	go p.udplRtcp.run()
	go p.tcpl.run()
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

					// if the publisher has disconnected
					// close all other connections that share the same path
					for oc := range p.clients {
						if oc.path == evt.client.path {
							go oc.close()
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

		case programEventClientGetStreamSdp:
			pub, ok := p.publishers[evt.path]
			if !ok {
				evt.res <- nil
				continue
			}
			evt.res <- pub.streamSdpText

		case programEventClientAnnounce:
			_, ok := p.publishers[evt.path]
			if ok {
				evt.res <- fmt.Errorf("another client is already publishing on path '%s'", evt.path)
				continue
			}

			evt.client.path = evt.path
			evt.client.streamSdpText = evt.sdpText
			evt.client.streamSdpParsed = evt.sdpParsed
			evt.client.state = _CLIENT_STATE_ANNOUNCE
			p.publishers[evt.path] = evt.client
			evt.res <- nil

		case programEventClientSetupPlay:
			pub, ok := p.publishers[evt.path]
			if !ok {
				evt.res <- fmt.Errorf("no one is streaming on path '%s'", evt.path)
				continue
			}

			if len(evt.client.streamTracks) >= len(pub.streamSdpParsed.Medias) {
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
			if !ok {
				evt.res <- fmt.Errorf("no one is streaming on path '%s'", evt.client.path)
				continue
			}

			if len(evt.client.streamTracks) != len(pub.streamSdpParsed.Medias) {
				evt.res <- fmt.Errorf("not all tracks have been setup")
				continue
			}

			evt.res <- nil

		case programEventClientPlay2:
			p.receiverCount += 1
			evt.client.state = _CLIENT_STATE_PLAY
			evt.res <- nil

		case programEventClientPause:
			p.receiverCount -= 1
			evt.client.state = _CLIENT_STATE_PRE_PLAY
			evt.res <- nil

		case programEventClientRecord:
			p.publisherCount += 1
			evt.client.state = _CLIENT_STATE_RECORD
			evt.res <- nil

		case programEventFrameUdp:
			// find publisher and track id from ip and port
			pub, trackId := func() (*serverClient, int) {
				for _, pub := range p.publishers {
					if pub.streamProtocol != _STREAM_PROTOCOL_UDP ||
						pub.state != _CLIENT_STATE_RECORD ||
						!pub.ip().Equal(evt.addr.IP) {
						continue
					}

					for i, t := range pub.streamTracks {
						if evt.trackFlowType == _TRACK_FLOW_RTP {
							if t.rtpPort == evt.addr.Port {
								return pub, i
							}
						} else {
							if t.rtcpPort == evt.addr.Port {
								return pub, i
							}
						}
					}
				}
				return nil, -1
			}()
			if pub == nil {
				continue
			}

			pub.udpLastFrameTime = time.Now()
			p.forwardTrack(pub.path, trackId, evt.trackFlowType, evt.buf)

		case programEventFrameTcp:
			p.forwardTrack(evt.path, evt.trackId, evt.trackFlowType, evt.buf)

		case programEventTerminate:
			break outer
		}
	}

	go func() {
		for rawEvt := range p.events {
			switch evt := rawEvt.(type) {
			case programEventClientClose:
				close(evt.done)

			case programEventClientGetStreamSdp:
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

			case programEventClientPause:
				evt.res <- fmt.Errorf("terminated")

			case programEventClientRecord:
				evt.res <- fmt.Errorf("terminated")
			}
		}
	}()

	p.tcpl.close()
	p.udplRtcp.close()
	p.udplRtp.close()

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

func (p *program) forwardTrack(path string, id int, trackFlowType trackFlowType, frame []byte) {
	for c := range p.clients {
		if c.path == path && c.state == _CLIENT_STATE_PLAY {
			if c.streamProtocol == _STREAM_PROTOCOL_UDP {
				if trackFlowType == _TRACK_FLOW_RTP {
					p.udplRtp.write <- &udpWrite{
						addr: &net.UDPAddr{
							IP:   c.ip(),
							Zone: c.zone(),
							Port: c.streamTracks[id].rtpPort,
						},
						buf: frame,
					}
				} else {
					p.udplRtcp.write <- &udpWrite{
						addr: &net.UDPAddr{
							IP:   c.ip(),
							Zone: c.zone(),
							Port: c.streamTracks[id].rtcpPort,
						},
						buf: frame,
					}
				}

			} else {
				c.write <- &gortsplib.InterleavedFrame{
					Channel: trackToInterleavedChannel(id, trackFlowType),
					Content: frame,
				}
			}
		}
	}
}

func main() {
	_, err := newProgram(os.Args[1:])
	if err != nil {
		log.Fatal("ERR: ", err)
	}

	select {}
}
