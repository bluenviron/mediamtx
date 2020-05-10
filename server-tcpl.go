package main

import (
	"log"
	"net"
	"sync"

	"github.com/aler9/gortsplib"
)

type serverTcpListener struct {
	p          *program
	nconn      *net.TCPListener
	mutex      sync.RWMutex
	clients    map[*serverClient]struct{}
	publishers map[string]*serverClient
	done       chan struct{}
}

func newServerTcpListener(p *program) (*serverTcpListener, error) {
	nconn, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.args.rtspPort,
	})
	if err != nil {
		return nil, err
	}

	l := &serverTcpListener{
		p:          p,
		nconn:      nconn,
		clients:    make(map[*serverClient]struct{}),
		publishers: make(map[string]*serverClient),
		done:       make(chan struct{}),
	}

	l.log("opened on :%d", p.args.rtspPort)
	return l, nil
}

func (l *serverTcpListener) log(format string, args ...interface{}) {
	log.Printf("[TCP listener] "+format, args...)
}

func (l *serverTcpListener) run() {
	for {
		nconn, err := l.nconn.AcceptTCP()
		if err != nil {
			break
		}

		newServerClient(l.p, nconn)
	}

	// close clients
	var doneChans []chan struct{}
	func() {
		l.mutex.Lock()
		defer l.mutex.Unlock()
		for c := range l.clients {
			c.close()
			doneChans = append(doneChans, c.done)
		}
	}()
	for _, c := range doneChans {
		<-c
	}

	close(l.done)
}

func (l *serverTcpListener) close() {
	l.nconn.Close()
	<-l.done
}

func (l *serverTcpListener) forwardTrack(path string, id int, flow trackFlow, frame []byte) {
	for c := range l.clients {
		if c.path == path && c.state == _CLIENT_STATE_PLAY {
			if c.streamProtocol == _STREAM_PROTOCOL_UDP {
				if flow == _TRACK_FLOW_RTP {
					l.p.udplRtp.write <- &udpWrite{
						addr: &net.UDPAddr{
							IP:   c.ip(),
							Zone: c.zone(),
							Port: c.streamTracks[id].rtpPort,
						},
						buf: frame,
					}
				} else {
					l.p.udplRtcp.write <- &udpWrite{
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
					Channel: trackToInterleavedChannel(id, flow),
					Content: frame,
				}
			}
		}
	}
}
