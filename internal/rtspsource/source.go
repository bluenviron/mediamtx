package rtspsource

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
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
	fingerprint     string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	readBufferCount int
	readBufferSize  int
	wg              *sync.WaitGroup
	stats           *stats.Stats
	parent          Parent

	ctx       context.Context
	ctxCancel func()
}

// New allocates a Source.
func New(
	ctxParent context.Context,
	ur string,
	proto *gortsplib.StreamProtocol,
	fingerprint string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	readBufferCount int,
	readBufferSize int,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Source {

	ctx, ctxCancel := context.WithCancel(ctxParent)

	s := &Source{
		ur:              ur,
		proto:           proto,
		fingerprint:     fingerprint,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
		readBufferSize:  readBufferSize,
		wg:              wg,
		stats:           stats,
		parent:          parent,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
	}

	atomic.AddInt64(s.stats.CountSourcesRTSP, +1)
	s.log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()
	return s
}

// Close closes a Source.
func (s *Source) Close() {
	atomic.AddInt64(s.stats.CountSourcesRTSP, -1)
	s.log(logger.Info, "stopped")
	s.ctxCancel()
}

// IsSource implements source.Source.
func (s *Source) IsSource() {}

// IsExtSource implements source.ExtSource.
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
			case <-s.ctx.Done():
				return false
			}
		}()
		if !ok {
			break
		}
	}

	s.ctxCancel()
}

func (s *Source) runInner() bool {
	s.log(logger.Debug, "connecting")

	client := &gortsplib.Client{
		StreamProtocol: s.proto,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				h := sha256.New()
				h.Write(cs.PeerCertificates[0].Raw)
				hstr := hex.EncodeToString(h.Sum(nil))
				fingerprintLower := strings.ToLower(s.fingerprint)

				if hstr != fingerprintLower {
					return fmt.Errorf("server fingerprint do not match: expected %s, got %s",
						fingerprintLower, hstr)
				}

				return nil
			},
		},
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

	innerCtx, innerCtxCancel := context.WithCancel(context.Background())

	var conn *gortsplib.ClientConn
	var err error
	dialDone := make(chan struct{})
	go func() {
		defer close(dialDone)
		conn, err = client.DialReadContext(innerCtx, s.ur)
	}()

	select {
	case <-s.ctx.Done():
		innerCtxCancel()
		<-dialDone
		return false

	case <-dialDone:
		innerCtxCancel()
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

	readErr := make(chan error)
	go func() {
		readErr <- conn.ReadFrames(func(trackID int, streamType gortsplib.StreamType, payload []byte) {
			res.SP.OnFrame(trackID, streamType, payload)
		})
	}()

	select {
	case <-s.ctx.Done():
		conn.Close()
		<-readErr
		return false

	case err := <-readErr:
		s.log(logger.Info, "ERR: %s", err)
		conn.Close()
		return true
	}
}
