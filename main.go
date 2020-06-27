package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"regexp"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	"gortc.io/sdp"
)

var Version = "v0.0.0"

func parseIpCidrList(in []string) ([]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}

	var ret []interface{}
	for _, t := range in {
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

type conf struct {
	Protocols    []string      `yaml:"protocols"`
	RtspPort     int           `yaml:"rtspPort"`
	RtpPort      int           `yaml:"rtpPort"`
	RtcpPort     int           `yaml:"rtcpPort"`
	PublishUser  string        `yaml:"publishUser"`
	PublishPass  string        `yaml:"publishPass"`
	PublishIps   []string      `yaml:"publishIps"`
	ReadUser     string        `yaml:"readUser"`
	ReadPass     string        `yaml:"readPass"`
	ReadIps      []string      `yaml:"readIps"`
	PreScript    string        `yaml:"preScript"`
	PostScript   string        `yaml:"postScript"`
	ReadTimeout  time.Duration `yaml:"readTimeout"`
	WriteTimeout time.Duration `yaml:"writeTimeout"`
	Pprof        bool          `yaml:"pprof"`
}

func loadConf(confPath string, stdin io.Reader) (*conf, error) {
	if confPath == "stdin" {
		var ret conf
		err := yaml.NewDecoder(stdin).Decode(&ret)
		if err != nil {
			return nil, err
		}

		return &ret, nil

	} else {
		// conf.yml is optional
		if confPath == "conf.yml" {
			if _, err := os.Stat(confPath); err != nil {
				return &conf{}, nil
			}
		}

		f, err := os.Open(confPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		var ret conf
		err = yaml.NewDecoder(f).Decode(&ret)
		if err != nil {
			return nil, err
		}

		return &ret, nil
	}
}

type program struct {
	conf           *conf
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

func newProgram(sargs []string, stdin io.Reader) (*program, error) {
	kingpin.CommandLine.Help = "rtsp-simple-server " + Version + "\n\n" +
		"RTSP server."

	argVersion := kingpin.Flag("version", "print version").Bool()
	argConfPath := kingpin.Arg("confpath", "path to a config file. The default is conf.yml. Use 'stdin' to read config from stdin").Default("conf.yml").String()

	kingpin.MustParse(kingpin.CommandLine.Parse(sargs))

	if *argVersion == true {
		fmt.Println(Version)
		os.Exit(0)
	}

	conf, err := loadConf(*argConfPath, stdin)
	if err != nil {
		return nil, err
	}

	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 5 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 5 * time.Second
	}

	if len(conf.Protocols) == 0 {
		conf.Protocols = []string{"udp", "tcp"}
	}
	protocols := make(map[streamProtocol]struct{})
	for _, proto := range conf.Protocols {
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

	if conf.RtspPort == 0 {
		conf.RtspPort = 8554
	}

	if conf.RtpPort == 0 {
		conf.RtpPort = 8000
	}
	if (conf.RtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}
	if conf.RtcpPort == 0 {
		conf.RtcpPort = 8001
	}
	if conf.RtcpPort != (conf.RtpPort + 1) {
		return nil, fmt.Errorf("rtcp and rtp ports must be consecutive")
	}

	if conf.PublishUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(conf.PublishUser) {
			return nil, fmt.Errorf("publish username must be alphanumeric")
		}
	}
	if conf.PublishPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(conf.PublishPass) {
			return nil, fmt.Errorf("publish password must be alphanumeric")
		}
	}
	publishIps, err := parseIpCidrList(conf.PublishIps)
	if err != nil {
		return nil, err
	}

	if conf.ReadUser != "" && conf.ReadPass == "" || conf.ReadUser == "" && conf.ReadPass != "" {
		return nil, fmt.Errorf("read username and password must be both filled")
	}
	if conf.ReadUser != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(conf.ReadUser) {
			return nil, fmt.Errorf("read username must be alphanumeric")
		}
	}
	if conf.ReadPass != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(conf.ReadPass) {
			return nil, fmt.Errorf("read password must be alphanumeric")
		}
	}
	if conf.ReadUser != "" && conf.ReadPass == "" || conf.ReadUser == "" && conf.ReadPass != "" {
		return nil, fmt.Errorf("read username and password must be both filled")
	}
	readIps, err := parseIpCidrList(conf.ReadIps)
	if err != nil {
		return nil, err
	}

	p := &program{
		conf:       conf,
		protocols:  protocols,
		publishIps: publishIps,
		readIps:    readIps,
		clients:    make(map[*serverClient]struct{}),
		publishers: make(map[string]*serverClient),
		events:     make(chan programEvent),
		done:       make(chan struct{}),
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

	p.udplRtp, err = newServerUdpListener(p, conf.RtpPort, _TRACK_FLOW_RTP)
	if err != nil {
		return nil, err
	}

	p.udplRtcp, err = newServerUdpListener(p, conf.RtcpPort, _TRACK_FLOW_RTCP)
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
					p.udplRtp.write(&net.UDPAddr{
						IP:   c.ip(),
						Zone: c.zone(),
						Port: c.streamTracks[id].rtpPort,
					}, frame)

				} else {
					p.udplRtcp.write(&net.UDPAddr{
						IP:   c.ip(),
						Zone: c.zone(),
						Port: c.streamTracks[id].rtcpPort,
					}, frame)
				}

			} else {
				c.writeFrame(trackToInterleavedChannel(id, trackFlowType), frame)
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
