package main

import (
	"log"
	"net"
	"time"
)

type udpWrite struct {
	addr *net.UDPAddr
	buf  []byte
}

type serverUdpListener struct {
	p         *program
	nconn     *net.UDPConn
	flow      trackFlow
	chanWrite chan *udpWrite
}

func newServerUdpListener(p *program, port int, flow trackFlow) (*serverUdpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &serverUdpListener{
		p:         p,
		nconn:     nconn,
		flow:      flow,
		chanWrite: make(chan *udpWrite),
	}

	l.log("opened on :%d", port)
	return l, nil
}

func (l *serverUdpListener) log(format string, args ...interface{}) {
	var label string
	if l.flow == _TRACK_FLOW_RTP {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	log.Printf("[UDP/"+label+" listener] "+format, args...)
}

func (l *serverUdpListener) run() {
	go func() {
		for {
			// create a buffer for each read.
			// this is necessary since the buffer is propagated with channels
			// so it must be unique.
			buf := make([]byte, 2048) // UDP MTU is 1400
			n, addr, err := l.nconn.ReadFromUDP(buf)
			if err != nil {
				l.log("ERR: %s", err)
				break
			}

			func() {
				l.p.mutex.RLock()
				defer l.p.mutex.RUnlock()

				// find path and track id from ip and port
				path, trackId := func() (string, int) {
					for _, pub := range l.p.publishers {
						for i, t := range pub.streamTracks {
							if !pub.ip().Equal(addr.IP) {
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
	}()

	go func() {
		for {
			w := <-l.chanWrite
			l.nconn.SetWriteDeadline(time.Now().Add(_WRITE_TIMEOUT))
			l.nconn.WriteTo(w.buf, w.addr)
		}
	}()
}
