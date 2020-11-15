package serverudp

import (
	"net"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/multibuffer"
)

const (
	readBufferSize = 2048
)

// Publisher is implemented by client.Client.
type Publisher interface {
	OnUdpPublisherFrame(int, base.StreamType, []byte)
}

type publisherData struct {
	publisher Publisher
	trackId   int
}

type bufAddrPair struct {
	buf  []byte
	addr *net.UDPAddr
}

// Parent is implemented by program.
type Parent interface {
	Log(string, ...interface{})
}

type publisherAddr struct {
	ip   [net.IPv6len]byte // use a fixed-size array to enable the equality operator
	port int
}

func (p *publisherAddr) fill(ip net.IP, port int) {
	p.port = port

	if len(ip) == net.IPv4len {
		copy(p.ip[0:], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff}) // v4InV6Prefix
		copy(p.ip[12:], ip)
	} else {
		copy(p.ip[:], ip)
	}
}

// Server is a RTSP UDP server.
type Server struct {
	writeTimeout time.Duration
	streamType   gortsplib.StreamType

	pc              *net.UDPConn
	readBuf         *multibuffer.MultiBuffer
	publishersMutex sync.RWMutex
	publishers      map[publisherAddr]*publisherData

	// in
	write chan bufAddrPair

	// out
	done chan struct{}
}

// New allocates a Server.
func New(writeTimeout time.Duration,
	port int,
	streamType gortsplib.StreamType,
	parent Parent) (*Server, error) {

	pc, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	s := &Server{
		writeTimeout: writeTimeout,
		streamType:   streamType,
		pc:           pc,
		readBuf:      multibuffer.New(2, readBufferSize),
		publishers:   make(map[publisherAddr]*publisherData),
		write:        make(chan bufAddrPair),
		done:         make(chan struct{}),
	}

	var label string
	if s.streamType == gortsplib.StreamTypeRtp {
		label = "RTP"
	} else {
		label = "RTCP"
	}
	parent.Log("[UDP/"+label+" server] opened on :%d", port)

	go s.run()
	return s, nil
}

// Close closes a Server.
func (s *Server) Close() {
	s.pc.Close()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for w := range s.write {
			s.pc.SetWriteDeadline(time.Now().Add(s.writeTimeout))
			s.pc.WriteTo(w.buf, w.addr)
		}
	}()

	for {
		buf := s.readBuf.Next()
		n, addr, err := s.pc.ReadFromUDP(buf)
		if err != nil {
			break
		}

		func() {
			s.publishersMutex.RLock()
			defer s.publishersMutex.RUnlock()

			// find publisher data
			var pubAddr publisherAddr
			pubAddr.fill(addr.IP, addr.Port)
			pubData, ok := s.publishers[pubAddr]
			if !ok {
				return
			}

			pubData.publisher.OnUdpPublisherFrame(pubData.trackId, s.streamType, buf[:n])
		}()
	}

	close(s.write)
	<-writeDone
}

// Port returns the server local port.
func (s *Server) Port() int {
	return s.pc.LocalAddr().(*net.UDPAddr).Port
}

// Write writes a UDP packet.
func (s *Server) Write(data []byte, addr *net.UDPAddr) {
	s.write <- bufAddrPair{data, addr}
}

// AddPublisher adds a publisher.
func (s *Server) AddPublisher(ip net.IP, port int, publisher Publisher, trackId int) {
	s.publishersMutex.Lock()
	defer s.publishersMutex.Unlock()

	var addr publisherAddr
	addr.fill(ip, port)

	s.publishers[addr] = &publisherData{
		publisher: publisher,
		trackId:   trackId,
	}
}

// RemovePublisher removes a publisher.
func (s *Server) RemovePublisher(ip net.IP, port int, publisher Publisher) {
	s.publishersMutex.Lock()
	defer s.publishersMutex.Unlock()

	var addr publisherAddr
	addr.fill(ip, port)

	delete(s.publishers, addr)
}
