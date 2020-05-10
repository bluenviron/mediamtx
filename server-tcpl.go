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
}

func newServerTcpListener(p *program) (*serverTcpListener, error) {
	nconn, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: p.args.rtspPort,
	})
	if err != nil {
		return nil, err
	}

	s := &serverTcpListener{
		p:          p,
		nconn:      nconn,
		clients:    make(map[*serverClient]struct{}),
		publishers: make(map[string]*serverClient),
	}

	s.log("opened on :%d", p.args.rtspPort)
	return s, nil
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

		rsc := newServerClient(l.p, nconn)
		go rsc.run()
	}
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
