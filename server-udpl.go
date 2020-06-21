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
	p     *program
	nconn *net.UDPConn
	flow  trackFlow
	write chan *udpWrite
	done  chan struct{}
}

func newServerUdpListener(p *program, port int, flow trackFlow) (*serverUdpListener, error) {
	nconn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	l := &serverUdpListener{
		p:     p,
		nconn: nconn,
		flow:  flow,
		write: make(chan *udpWrite),
		done:  make(chan struct{}),
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
		for w := range l.write {
			l.nconn.SetWriteDeadline(time.Now().Add(l.p.args.writeTimeout))
			l.nconn.WriteTo(w.buf, w.addr)
		}
	}()

	for {
		// create a buffer for each read.
		// this is necessary since the buffer is propagated with channels
		// so it must be unique.
		buf := make([]byte, 2048) // UDP MTU is 1400
		n, addr, err := l.nconn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		func() {
			l.p.tcpl.mutex.Lock()
			defer l.p.tcpl.mutex.Unlock()

			// find publisher and track id from ip and port
			pub, trackId := func() (*serverClient, int) {
				for _, pub := range l.p.tcpl.publishers {
					if pub.streamProtocol != _STREAM_PROTOCOL_UDP ||
						pub.state != _CLIENT_STATE_RECORD ||
						!pub.ip().Equal(addr.IP) {
						continue
					}

					for i, t := range pub.streamTracks {
						if l.flow == _TRACK_FLOW_RTP {
							if t.rtpPort == addr.Port {
								return pub, i
							}
						} else {
							if t.rtcpPort == addr.Port {
								return pub, i
							}
						}
					}
				}
				return nil, -1
			}()
			if pub == nil {
				return
			}

			pub.udpLastFrameTime = time.Now()
			l.p.tcpl.forwardTrack(pub.path, trackId, l.flow, buf[:n])
		}()
	}

	close(l.write)

	close(l.done)
}

func (l *serverUdpListener) close() {
	l.nconn.Close()
	<-l.done
}
