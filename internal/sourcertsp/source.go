package sourcertsp

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/source"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	retryPause = 5 * time.Second
)

// Parent is implemented by path.Path.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnExtSourceSetReady(req source.ExtSetReadyReq)
	OnExtSourceSetNotReady(req source.ExtSetNotReadyReq)
}

// Source is a RTSP external source.
type Source struct {
	ur              string
	proto           *gortsplib.StreamProtocol
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	readBufferSize  int
	wg              *sync.WaitGroup
	stats           *stats.Stats
	parent          Parent

	// in
	terminate chan struct{}
}

// New allocates a Source.
func New(ur string,
	proto *gortsplib.StreamProtocol,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Source {
	s := &Source{
		ur:              ur,
		proto:           proto,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
		readBufferSize:  readBufferSize,
		wg:              wg,
		stats:           stats,
		parent:          parent,
		terminate:       make(chan struct{}),
	}

	atomic.AddInt64(s.stats.CountSourcesRtsp, +1)
	s.log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()
	return s
}

// Close closes a Source.
func (s *Source) Close() {
	atomic.AddInt64(s.stats.CountSourcesRtsp, -1)
	s.log(logger.Info, "stopped")
	close(s.terminate)
}

// IsSource implements source.Source.
func (s *Source) IsSource() {}

// IsExtSource implements path.extSource.
func (s *Source) IsExtSource() {}

func (s *Source) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[rtsp source] "+format, args...)
}

func (s *Source) run() {
	defer s.wg.Done()

	for {
		ok := func() bool {
			ok := s.runInner()
			if !ok {
				return false
			}

			select {
			case <-time.After(retryPause):
				return true
			case <-s.terminate:
				return false
			}
		}()
		if !ok {
			break
		}
	}
}

func (s *Source) runInner() bool {
	s.log(logger.Info, "connecting")

	var conn *gortsplib.ClientConn
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)

		conf := gortsplib.ClientConf{
			StreamProtocol:  s.proto,
			ReadTimeout:     s.readTimeout,
			WriteTimeout:    s.writeTimeout,
			ReadBufferCount: s.readBufferCount,
			ReadBufferSize:  s.readBufferSize,
			OnRequest: func(req *base.Request) {
				s.log(logger.Debug, "c->s %v", req)
			},
			OnResponse: func(res *base.Response) {
				s.log(logger.Debug, "s->c %v", res)
			},
		}
		conn, err = conf.DialRead(s.ur)
	}()

	select {
	case <-s.terminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.log(logger.Info, "ERR: %s", err)
		return true
	}

	s.log(logger.Info, "ready")

	cres := make(chan source.ExtSetReadyRes)
	s.parent.OnExtSourceSetReady(source.ExtSetReadyReq{
		Tracks: conn.Tracks(),
		Res:    cres,
	})
	res := <-cres

	defer func() {
		res := make(chan struct{})
		s.parent.OnExtSourceSetNotReady(source.ExtSetNotReadyReq{
			Res: res,
		})
		<-res
	}()

	done := conn.ReadFrames(func(trackID int, streamType gortsplib.StreamType, payload []byte) {
		res.SP.OnFrame(trackID, streamType, payload)
	})

	for {
		select {
		case <-s.terminate:
			conn.Close()
			<-done
			return false

		case err := <-done:
			conn.Close()
			s.log(logger.Info, "ERR: %s", err)
			return true
		}
	}
}
