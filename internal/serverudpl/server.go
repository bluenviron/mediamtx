package serverudpl

import (
	"strconv"

	"github.com/aler9/gortsplib"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

// Parent is implemented by program.
type Parent interface {
	Log(logger.Level, string, ...interface{})
}

// New allocates a gortsplib.ServerUDPListener.
func New(
	listenIP string,
	port int,
	streamType gortsplib.StreamType,
	parent Parent) (*gortsplib.ServerUDPListener, error) {

	address := listenIP + ":" + strconv.FormatInt(int64(port), 10)
	listener, err := gortsplib.NewServerUDPListener(address)
	if err != nil {
		return nil, err
	}

	label := func() string {
		if streamType == gortsplib.StreamTypeRTP {
			return "RTP"
		}
		return "RTCP"
	}()
	parent.Log(logger.Info, "[UDP/"+label+" listener] opened on %s", address)

	return listener, nil
}
