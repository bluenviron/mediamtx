package main

import (
	"log"
	"net"
)

type udpListener struct {
	p     *program
	nconn *net.UDPConn
	flow  trackFlow
}

func newUdpListener(p *program, port int, flow trackFlow) (*udpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &udpListener{
		p:     p,
		nconn: nconn,
		flow:  flow,
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *udpListener) log(format string, args ...interface{}) {
	var label string
	if l.flow == _TRACK_FLOW_RTP {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	log.Printf("["+label+" listener] "+format, args...)
}

func (l *udpListener) run() {
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

			if l.p.publisher == nil {
				return
			}

			if l.p.publisher.streamProtocol != _STREAM_PROTOCOL_UDP {
				return
			}

			if !l.p.publisher.ip.Equal(addr.IP) {
				return
			}

			// get track id by using client port
			trackId := func() int {
				for i, t := range l.p.publisher.streamTracks {
					if l.flow == _TRACK_FLOW_RTP {
						if t.rtpPort == addr.Port {
							return i
						}
					} else {
						if t.rtcpPort == addr.Port {
							return i
						}
					}
				}
				return -1
			}()
			if trackId < 0 {
				return
			}

			l.p.forwardTrack(l.flow, trackId, buf[:n])
		}()
	}
}
