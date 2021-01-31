package serverrtmp

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/notedit/rtmp/format/rtmp"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	codecH264 = 7
	codecAAC  = 10
)

func readMetadata(conn *rtmp.Conn) (flvio.AMFMap, error) {
	pkt, err := conn.ReadPacket()
	if err != nil {
		return nil, err
	}

	if pkt.Type != av.Metadata {
		return nil, fmt.Errorf("first packet must be metadata")
	}

	arr, err := flvio.ParseAMFVals(pkt.Data, false)
	if err != nil {
		return nil, err
	}

	if len(arr) != 1 {
		return nil, fmt.Errorf("invalid metadata")
	}

	ma, ok := arr[0].(flvio.AMFMap)
	if !ok {
		return nil, fmt.Errorf("invalid metadata")
	}

	return ma, nil
}

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// Server is a RTMP listener.
type Server struct {
	l   net.Listener
	srv *rtmp.Server
	wg  sync.WaitGroup
}

// New allocates a Server.
func New(
	listenIP string,
	port int,
	parent Parent) (*Server, error) {

	address := listenIP + ":" + strconv.FormatInt(int64(port), 10)
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	s := &Server{
		l: l,
	}

	s.srv = rtmp.NewServer()
	s.srv.HandleConn = s.innerHandleConn

	parent.Log(logger.Info, "[RTMP listener] opened on %s", address)

	s.wg.Add(1)
	go s.run()

	return s, nil
}

// Close closes a Server.
func (s *Server) Close() {
	s.l.Close()
	s.wg.Wait()
}

func (s *Server) run() {
	defer s.wg.Done()

	for {
		nconn, err := s.l.Accept()
		if err != nil {
			break
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.srv.HandleNetConn(nconn)
		}()
	}
}

func (s *Server) innerHandleConn(rconn *rtmp.Conn, nconn net.Conn) {
	defer nconn.Close()

	err := func() error {
		if !rconn.Publishing {
			return fmt.Errorf("not publishing")
		}

		return nil
	}()
	fmt.Println("ERR", err)
}
