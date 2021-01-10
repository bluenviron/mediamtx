package sourcertsp

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	retryPause = 5 * time.Second
)

// Parent is implemented by path.Path.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnSourceSetReady(gortsplib.Tracks)
	OnSourceSetNotReady()
	OnFrame(int, gortsplib.StreamType, []byte)
}

// Source is a RTSP source.
type Source struct {
	ur              string
	proto           *gortsplib.StreamProtocol
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount uint64
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
	readBufferCount uint64,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Source {
	s := &Source{
		ur:              ur,
		proto:           proto,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
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

// IsSource implements path.source.
func (s *Source) IsSource() {}

// IsSourceExternal implements path.sourceExternal.
func (s *Source) IsSourceExternal() {}

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

			t := time.NewTimer(retryPause)
			defer t.Stop()

			select {
			case <-t.C:
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

	tracks := conn.Tracks()

	s.log(logger.Info, "ready")
	s.parent.OnSourceSetReady(tracks)
	defer s.parent.OnSourceSetNotReady()

	done := conn.ReadFrames(func(trackID int, streamType gortsplib.StreamType, payload []byte) {
		s.parent.OnFrame(trackID, streamType, payload)
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
