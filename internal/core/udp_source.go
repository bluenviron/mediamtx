package core

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/multicast"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	// same size as GStreamer's rtspsrc
	udpKernelReadBufferSize = 0x80000
)

type packetConnReader struct {
	net.PacketConn
}

func newPacketConnReader(pc net.PacketConn) *packetConnReader {
	return &packetConnReader{
		PacketConn: pc,
	}
}

func (r *packetConnReader) Read(p []byte) (int, error) {
	n, _, err := r.PacketConn.ReadFrom(p)
	return n, err
}

type packetConn interface {
	net.PacketConn
	SetReadBuffer(int) error
}

type udpSourceParent interface {
	logger.Writer
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
}

type udpSource struct {
	readTimeout conf.StringDuration
	parent      udpSourceParent
}

func newUDPSource(
	readTimeout conf.StringDuration,
	parent udpSourceParent,
) *udpSource {
	return &udpSource{
		readTimeout: readTimeout,
		parent:      parent,
	}
}

func (s *udpSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[UDP source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *udpSource) run(ctx context.Context, cnf *conf.Path, _ chan *conf.Path) error {
	s.Log(logger.Debug, "connecting")

	hostPort := cnf.Source[len("udp://"):]

	addr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return err
	}

	var pc packetConn

	if ip4 := addr.IP.To4(); ip4 != nil && addr.IP.IsMulticast() {
		pc, err = multicast.NewMultiConn(hostPort, true, net.ListenPacket)
		if err != nil {
			return err
		}
	} else {
		tmp, err := net.ListenPacket(restrictNetwork("udp", addr.String()))
		if err != nil {
			return err
		}
		pc = tmp.(*net.UDPConn)
	}

	defer pc.Close()

	err = pc.SetReadBuffer(udpKernelReadBufferSize)
	if err != nil {
		return err
	}

	readerErr := make(chan error)
	go func() {
		readerErr <- s.runReader(pc)
	}()

	select {
	case err := <-readerErr:
		return err

	case <-ctx.Done():
		pc.Close()
		<-readerErr
		return fmt.Errorf("terminated")
	}
}

func (s *udpSource) runReader(pc net.PacketConn) error {
	pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
	r, err := mpegts.NewReader(mpegts.NewBufferedReader(newPacketConnReader(pc)))
	if err != nil {
		return err
	}

	decodeErrLogger := logger.NewLimitedLogger(s)

	r.OnDecodeError(func(err error) {
		decodeErrLogger.Log(logger.Warn, err.Error())
	})

	var stream *stream.Stream

	medias, err := mpegtsSetupTracks(r, &stream)
	if err != nil {
		return err
	}

	res := s.parent.setReady(pathSourceStaticSetReadyReq{
		desc:               &description.Session{Medias: medias},
		generateRTPPackets: true,
	})
	if res.err != nil {
		return res.err
	}

	defer s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})

	stream = res.stream

	for {
		pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*udpSource) apiSourceDescribe() apiPathSourceOrReader {
	return apiPathSourceOrReader{
		Type: "udpSource",
		ID:   "",
	}
}
