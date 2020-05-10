package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

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

type args struct {
	version      bool
	protocolsStr string
	rtspPort     int
	rtpPort      int
	rtcpPort     int
	publishUser  string
	publishPass  string
	preScript    string
	postScript   string
}

type program struct {
	args      args
	protocols map[streamProtocol]struct{}
	tcpl      *serverTcpListener
	udplRtp   *serverUdpListener
	udplRtcp  *serverUdpListener
}

func newProgram(args args) (*program, error) {
	if args.protocolsStr == "" {
		args.protocolsStr = "udp,tcp"
	}
	if args.rtspPort == 0 {
		args.rtspPort = 8554
	}
	if args.rtpPort == 0 {
		args.rtpPort = 8000
	}
	if args.rtcpPort == 0 {
		args.rtcpPort = 8001
	}

	if args.version == true {
		fmt.Println("rtsp-simple-server " + Version)
		os.Exit(0)
	}

	if (args.rtpPort % 2) != 0 {
		return nil, fmt.Errorf("rtp port must be even")
	}

	if args.rtcpPort != (args.rtpPort + 1) {
		return nil, fmt.Errorf("rtcp and rtp ports must be consecutive")
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

	if args.publishUser != "" && args.publishPass == "" || args.publishUser == "" && args.publishPass != "" {
		return nil, fmt.Errorf("publish username and password must be both filled")
	}

	log.Printf("rtsp-simple-server %s", Version)

	p := &program{
		args:      args,
		protocols: protocols,
	}

	var err error

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

	return p, nil
}

func (p *program) close() {
	p.tcpl.close()
	p.udplRtcp.close()
	p.udplRtp.close()
}

func main() {
	kingpin.CommandLine.Help = "rtsp-simple-server " + Version + "\n\n" +
		"RTSP server."

	argVersion := kingpin.Flag("version", "print rtsp-simple-server version").Bool()
	argProtocolsStr := kingpin.Flag("protocols", "supported protocols").Default("udp,tcp").String()
	argRtspPort := kingpin.Flag("rtsp-port", "port of the RTSP TCP listener").Default("8554").Int()
	argRtpPort := kingpin.Flag("rtp-port", "port of the RTP UDP listener").Default("8000").Int()
	argRtcpPort := kingpin.Flag("rtcp-port", "port of the RTCP UDP listener").Default("8001").Int()
	argPublishUser := kingpin.Flag("publish-user", "optional username required to publish").Default("").String()
	argPublishPass := kingpin.Flag("publish-pass", "optional password required to publish").Default("").String()
	argPreScript := kingpin.Flag("pre-script", "optional script to run on client connect").Default("").String()
	argPostScript := kingpin.Flag("post-script", "optional script to run on client disconnect").Default("").String()

	kingpin.Parse()

	_, err := newProgram(args{
		version:      *argVersion,
		protocolsStr: *argProtocolsStr,
		rtspPort:     *argRtspPort,
		rtpPort:      *argRtpPort,
		rtcpPort:     *argRtcpPort,
		publishUser:  *argPublishUser,
		publishPass:  *argPublishPass,
		preScript:    *argPreScript,
		postScript:   *argPostScript,
	})
	if err != nil {
		log.Fatal("ERR: ", err)
	}

	infty := make(chan struct{})
	<-infty
}
