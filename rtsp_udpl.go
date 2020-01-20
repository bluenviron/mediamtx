package main

import (
	"log"
	"net"
)

type rtspUdpListener struct {
	p     *program
	nconn *net.UDPConn
	flow  trackFlow
}

func newRtspUdpListener(p *program, port int, flow trackFlow) (*rtspUdpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &rtspUdpListener{
		p:     p,
		nconn: nconn,
		flow:  flow,
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *rtspUdpListener) log(format string, args ...interface{}) {
	var label string
	if l.flow == _TRACK_FLOW_RTP {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	log.Printf("[RTSP UDP/"+label+" listener] "+format, args...)
}

func (l *rtspUdpListener) run() {
	buf := make([]byte, 2048) // UDP MTU is 1400

	for {
		n, addr, err := l.nconn.ReadFromUDP(buf)
		if err != nil {
			l.log("ERR: %s", err)
			break
		}

		func() {
			l.p.mutex.RLock()
			defer l.p.mutex.RUnlock()

			// find path and track id
			path, trackId := func() (string, int) {
				for _, pub := range l.p.publishers {
					for i, t := range pub.streamTracks {
						if !pub.ip.Equal(addr.IP) {
							continue
						}

						if l.flow == _TRACK_FLOW_RTP {
							if t.rtpPort == addr.Port {
								return pub.path, i
							}
						} else {
							if t.rtcpPort == addr.Port {
								return pub.path, i
							}
						}
					}
				}
				return "", -1
			}()
			if path == "" {
				return
			}

			l.p.forwardTrack(path, trackId, l.flow, buf[:n])
		}()
	}
}
