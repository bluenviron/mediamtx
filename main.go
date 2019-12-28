package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"gopkg.in/alecthomas/kingpin.v2"
)

var Version string = "v0.0.0"

type program struct {
	rtspPort     int
	rtpPort      int
	rtcpPort     int
	mutex        sync.Mutex
	rtspl        *rtspListener
	rtpl         *udpListener
	rtcpl        *udpListener
	clients      map[*rtspClient]struct{}
	streamAuthor *rtspClient
	streamSdp    []byte
}

func newProgram(rtspPort int, rtpPort int, rtcpPort int) (*program, error) {
	p := &program{
		rtspPort: rtspPort,
		rtpPort:  rtpPort,
		rtcpPort: rtcpPort,
		clients:  make(map[*rtspClient]struct{}),
	}

	var err error

	p.rtpl, err = newUdpListener(rtpPort, "RTP", func(l *udpListener, buf []byte) {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		for c := range p.clients {
			if c.state == "PLAY" {
				l.nconn.WriteTo(buf, &net.UDPAddr{
					IP:   c.IP,
					Port: c.rtpPort,
				})
			}
		}
	})
	if err != nil {
		return nil, err
	}

	p.rtcpl, err = newUdpListener(rtcpPort, "RTCP", func(l *udpListener, buf []byte) {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		for c := range p.clients {
			if c.state == "PLAY" {
				l.nconn.WriteTo(buf, &net.UDPAddr{
					IP:   c.IP,
					Port: c.rtcpPort,
				})
			}
		}
	})
	if err != nil {
		return nil, err
	}

	p.rtspl, err = newRtspListener(p)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *program) run() {
	var wg sync.WaitGroup

	wg.Add(1)
	go p.rtpl.run(wg)

	wg.Add(1)
	go p.rtcpl.run(wg)

	wg.Add(1)
	go p.rtspl.run(wg)

	wg.Wait()
}

func main() {
	kingpin.CommandLine.Help = "rtsp-simple-server " + Version + "\n\n" +
		"RTSP server."

	version := kingpin.Flag("version", "print rtsp-simple-server version").Bool()

	rtspPort := kingpin.Flag("rtsp-port", "port of the RTSP TCP listener").Default("8554").Int()
	rtpPort := kingpin.Flag("rtp-port", "port of the RTP UDP listener").Default("8000").Int()
	rtcpPort := kingpin.Flag("rtcp-port", "port of the RTCP UDP listener").Default("8001").Int()

	kingpin.Parse()

	if *version == true {
		fmt.Println("rtsp-simple-server " + Version)
		os.Exit(0)
	}

	p, err := newProgram(*rtspPort, *rtpPort, *rtcpPort)
	if err != nil {
		log.Fatal("ERR:", err)
	}

	p.run()
}
