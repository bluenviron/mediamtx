package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"gopkg.in/alecthomas/kingpin.v2"
)

var Version string = "v0.0.0"

const (
	_READ_TIMEOUT  = 5 * time.Second
	_WRITE_TIMEOUT = 5 * time.Second
)

type trackFlow int

const (
	_TRACK_FLOW_RTP trackFlow = iota
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

type program struct {
	protocols  map[streamProtocol]struct{}
	rtspPort   int
	rtpPort    int
	rtcpPort   int
	publishKey string
	mutex      sync.RWMutex
	rtspl      *serverTcpListener
	rtpl       *serverUdpListener
	rtcpl      *serverUdpListener
	clients    map[*client]struct{}
	publishers map[string]*client
}

func newProgram(protocolsStr string, rtspPort int, rtpPort int, rtcpPort int, publishKey string) (*program, error) {

	if rtspPort == 0 {
		return nil, fmt.Errorf("rtsp port not provided")
	}

	if rtpPort == 0 {
		return nil, fmt.Errorf("rtp port not provided")
	}

	if rtcpPort == 0 {
		return nil, fmt.Errorf("rtcp port not provided")
	}

	if (rtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}

	if rtcpPort != (rtpPort + 1) {
		return nil, fmt.Errorf("rtcp port must be rtp port plus 1")
	}

	protocols := make(map[streamProtocol]struct{})
	for _, proto := range strings.Split(protocolsStr, ",") {
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

	if publishKey != "" {
		if !regexp.MustCompile("^[a-zA-Z0-9]+$").MatchString(publishKey) {
			return nil, fmt.Errorf("publish key must be alphanumeric")
		}
	}

	log.Printf("rtsp-simple-server %s", Version)

	p := &program{
		protocols:  protocols,
		rtspPort:   rtspPort,
		rtpPort:    rtpPort,
		rtcpPort:   rtcpPort,
		publishKey: publishKey,
		clients:    make(map[*client]struct{}),
		publishers: make(map[string]*client),
	}

	var err error

	p.rtpl, err = newServerUdpListener(p, rtpPort, _TRACK_FLOW_RTP)
	if err != nil {
		return nil, err
	}

	p.rtcpl, err = newServerUdpListener(p, rtcpPort, _TRACK_FLOW_RTCP)
	if err != nil {
		return nil, err
	}

	p.rtspl, err = newServerTcpListener(p)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *program) run() {
	go p.rtpl.run()
	go p.rtcpl.run()
	go p.rtspl.run()

	infty := make(chan struct{})
	<-infty
}

func (p *program) forwardTrack(path string, id int, flow trackFlow, frame []byte) {
	for c := range p.clients {
		if c.path == path && c.state == _CLIENT_STATE_PLAY {
			if c.streamProtocol == _STREAM_PROTOCOL_UDP {
				if flow == _TRACK_FLOW_RTP {
					p.rtpl.nconn.SetWriteDeadline(time.Now().Add(_WRITE_TIMEOUT))
					p.rtpl.nconn.WriteTo(frame, &net.UDPAddr{
						IP:   c.ip,
						Port: c.streamTracks[id].rtpPort,
					})
				} else {
					p.rtcpl.nconn.SetWriteDeadline(time.Now().Add(_WRITE_TIMEOUT))
					p.rtcpl.nconn.WriteTo(frame, &net.UDPAddr{
						IP:   c.ip,
						Port: c.streamTracks[id].rtcpPort,
					})
				}

			} else {
				c.conn.NetConn().SetWriteDeadline(time.Now().Add(_WRITE_TIMEOUT))
				c.conn.WriteInterleavedFrame(&gortsplib.InterleavedFrame{
					Channel: trackToInterleavedChannel(id, flow),
					Content: frame,
				})
			}
		}
	}
}

func main() {
	kingpin.CommandLine.Help = "rtsp-simple-server " + Version + "\n\n" +
		"RTSP server."

	version := kingpin.Flag("version", "print rtsp-simple-server version").Bool()
	protocols := kingpin.Flag("protocols", "supported protocols").Default("udp,tcp").String()
	rtspPort := kingpin.Flag("rtsp-port", "port of the RTSP TCP listener").Default("8554").Int()
	rtpPort := kingpin.Flag("rtp-port", "port of the RTP UDP listener").Default("8000").Int()
	rtcpPort := kingpin.Flag("rtcp-port", "port of the RTCP UDP listener").Default("8001").Int()
	publishKey := kingpin.Flag("publish-key", "optional authentication key required to publish").Default("").String()

	kingpin.Parse()

	if *version == true {
		fmt.Println("rtsp-simple-server " + Version)
		os.Exit(0)
	}

	p, err := newProgram(*protocols, *rtspPort, *rtpPort, *rtcpPort, *publishKey)
	if err != nil {
		log.Fatal("ERR: ", err)
	}

	p.run()
}
