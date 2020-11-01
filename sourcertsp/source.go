package sourcertsp

import (
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"

	"github.com/aler9/rtsp-simple-server/stats"
)

const (
	retryPause = 5 * time.Second
)

type Parent interface {
	Log(string, ...interface{})
	OnSourceSetReady(gortsplib.Tracks)
	OnSourceSetNotReady()
	OnFrame(int, gortsplib.StreamType, []byte)
}

type Source struct {
	ur           string
	proto        gortsplib.StreamProtocol
	readTimeout  time.Duration
	writeTimeout time.Duration
	wg           *sync.WaitGroup
	stats        *stats.Stats
	parent       Parent

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

func New(ur string,
	proto gortsplib.StreamProtocol,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Source {
	s := &Source{
		ur:           ur,
		proto:        proto,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		wg:           wg,
		stats:        stats,
		parent:       parent,
		terminate:    make(chan struct{}),
	}

	atomic.AddInt64(s.stats.CountSourcesRtsp, +1)
	s.parent.Log("rtsp source started")

	s.wg.Add(1)
	go s.run()
	return s
}

func (s *Source) Close() {
	atomic.AddInt64(s.stats.CountSourcesRtsp, -1)
	s.parent.Log("rtsp source stopped")
	close(s.terminate)
}

func (s *Source) IsSource() {}

func (s *Source) IsSourceExternal() {}

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
	s.parent.Log("connecting to rtsp source")

	u, _ := url.Parse(s.ur)

	var conn *gortsplib.ConnClient
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)
		conn, err = gortsplib.NewConnClient(gortsplib.ConnClientConf{
			Host:            u.Host,
			ReadTimeout:     s.readTimeout,
			WriteTimeout:    s.writeTimeout,
			ReadBufferCount: 2,
		})
	}()

	select {
	case <-s.terminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.parent.Log("rtsp source ERR: %s", err)
		return true
	}

	_, err = conn.Options(u)
	if err != nil {
		conn.Close()
		s.parent.Log("rtsp source ERR: %s", err)
		return true
	}

	tracks, _, err := conn.Describe(u)
	if err != nil {
		conn.Close()
		s.parent.Log("rtsp source ERR: %s", err)
		return true
	}

	if s.proto == gortsplib.StreamProtocolUDP {
		return s.runUDP(u, conn, tracks)
	} else {
		return s.runTCP(u, conn, tracks)
	}
}

func (s *Source) runUDP(u *url.URL, conn *gortsplib.ConnClient, tracks gortsplib.Tracks) bool {
	for _, track := range tracks {
		_, err := conn.SetupUDP(u, gortsplib.TransportModePlay, track, 0, 0)
		if err != nil {
			conn.Close()
			s.parent.Log("rtsp source ERR: %s", err)
			return true
		}
	}

	_, err := conn.Play(u)
	if err != nil {
		conn.Close()
		s.parent.Log("rtsp source ERR: %s", err)
		return true
	}

	s.parent.Log("rtsp source ready")
	s.parent.OnSourceSetReady(tracks)

	var wg sync.WaitGroup

	// receive RTP packets
	for trackId := range tracks {
		wg.Add(1)
		go func(trackId int) {
			defer wg.Done()

			for {
				buf, err := conn.ReadFrameUDP(trackId, gortsplib.StreamTypeRtp)
				if err != nil {
					break
				}

				s.parent.OnFrame(trackId, gortsplib.StreamTypeRtp, buf)
			}
		}(trackId)
	}

	// receive RTCP packets
	for trackId := range tracks {
		wg.Add(1)
		go func(trackId int) {
			defer wg.Done()

			for {
				buf, err := conn.ReadFrameUDP(trackId, gortsplib.StreamTypeRtcp)
				if err != nil {
					break
				}

				s.parent.OnFrame(trackId, gortsplib.StreamTypeRtcp, buf)
			}
		}(trackId)
	}

	tcpConnDone := make(chan error)
	go func() {
		tcpConnDone <- conn.LoopUDP()
	}()

	var ret bool

outer:
	for {
		select {
		case <-s.terminate:
			conn.Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			conn.Close()
			s.parent.Log("rtsp source ERR: %s", err)
			ret = true
			break outer
		}
	}

	wg.Wait()

	s.parent.OnSourceSetNotReady()

	return ret
}

func (s *Source) runTCP(u *url.URL, conn *gortsplib.ConnClient, tracks gortsplib.Tracks) bool {
	for _, track := range tracks {
		_, err := conn.SetupTCP(u, gortsplib.TransportModePlay, track)
		if err != nil {
			conn.Close()
			s.parent.Log("rtsp source ERR: %s", err)
			return true
		}
	}

	_, err := conn.Play(u)
	if err != nil {
		conn.Close()
		s.parent.Log("rtsp source ERR: %s", err)
		return true
	}

	s.parent.Log("rtsp source ready")
	s.parent.OnSourceSetReady(tracks)

	tcpConnDone := make(chan error)
	go func() {
		for {
			trackId, streamType, content, err := conn.ReadFrameTCP()
			if err != nil {
				tcpConnDone <- err
				return
			}

			s.parent.OnFrame(trackId, streamType, content)
		}
	}()

	var ret bool

outer:
	for {
		select {
		case <-s.terminate:
			conn.Close()
			<-tcpConnDone
			ret = false
			break outer

		case err := <-tcpConnDone:
			conn.Close()
			s.parent.Log("rtsp source ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.parent.OnSourceSetNotReady()

	return ret
}
