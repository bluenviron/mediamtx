package core

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/pion/rtp"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

const (
	rtspSourceRetryPause = 5 * time.Second
)

type rtspSourceParent interface {
	log(logger.Level, string, ...interface{})
	onSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	onSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type rtspSource struct {
	ur              string
	proto           conf.SourceProtocol
	anyPortEnable   bool
	fingerprint     string
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	readBufferSize  int
	wg              *sync.WaitGroup
	parent          rtspSourceParent

	ctx       context.Context
	ctxCancel func()
}

func newRTSPSource(
	parentCtx context.Context,
	ur string,
	proto conf.SourceProtocol,
	anyPortEnable bool,
	fingerprint string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	readBufferSize int,
	wg *sync.WaitGroup,
	parent rtspSourceParent) *rtspSource {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtspSource{
		ur:              ur,
		proto:           proto,
		anyPortEnable:   anyPortEnable,
		fingerprint:     fingerprint,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
		readBufferSize:  readBufferSize,
		wg:              wg,
		parent:          parent,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
	}

	s.log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()

	return s
}

func (s *rtspSource) close() {
	s.log(logger.Info, "stopped")
	s.ctxCancel()
}

func (s *rtspSource) log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[rtsp source] "+format, args...)
}

func (s *rtspSource) run() {
	defer s.wg.Done()

	for {
		ok := func() bool {
			ok := s.runInner()
			if !ok {
				return false
			}

			select {
			case <-time.After(rtspSourceRetryPause):
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

func (s *rtspSource) runInner() bool {
	s.log(logger.Debug, "connecting")

	var tlsConfig *tls.Config

	if s.fingerprint != "" {
		tlsConfig = &tls.Config{
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
		}
	}

	c := &gortsplib.Client{
		Transport:       s.proto.Transport,
		TLSConfig:       tlsConfig,
		ReadTimeout:     time.Duration(s.readTimeout),
		WriteTimeout:    time.Duration(s.writeTimeout),
		ReadBufferCount: s.readBufferCount,
		ReadBufferSize:  s.readBufferSize,
		AnyPortEnable:   s.anyPortEnable,
		OnRequest: func(req *base.Request) {
			s.log(logger.Debug, "c->s %v", req)
		},
		OnResponse: func(res *base.Response) {
			s.log(logger.Debug, "s->c %v", res)
		},
	}

	u, err := base.ParseURL(s.ur)
	if err != nil {
		s.log(logger.Info, "ERR: %s", err)
		return true
	}

	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		s.log(logger.Info, "ERR: %s", err)
		return true
	}
	defer c.Close()

	readErr := make(chan error)
	go func() {
		readErr <- func() error {
			tracks, baseURL, _, err := c.Describe(u)
			if err != nil {
				return err
			}

			for _, t := range tracks {
				_, err := c.Setup(true, t, baseURL, 0, 0)
				if err != nil {
					return err
				}
			}

			err = s.handleMissingH264Params(c, tracks)
			if err != nil {
				return err
			}

			res := s.parent.onSourceStaticSetReady(pathSourceStaticSetReadyReq{
				source: s,
				tracks: c.Tracks(),
			})
			if res.err != nil {
				return res.err
			}

			s.log(logger.Info, "ready")

			defer func() {
				s.parent.onSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{source: s})
			}()

			c.OnPacketRTP = func(trackID int, pkt *rtp.Packet) {
				res.stream.writePacketRTP(trackID, pkt)
			}

			_, err = c.Play(nil)
			if err != nil {
				return err
			}

			return c.Wait()
		}()
	}()

	select {
	case err := <-readErr:
		s.log(logger.Info, "ERR: %s", err)
		return true

	case <-s.ctx.Done():
		c.Close()
		<-readErr
		return false
	}
}

func (s *rtspSource) handleMissingH264Params(c *gortsplib.Client, tracks gortsplib.Tracks) error {
	h264Track, h264TrackID := func() (*gortsplib.TrackH264, int) {
		for i, t := range tracks {
			if th264, ok := t.(*gortsplib.TrackH264); ok {
				if th264.SPS() == nil {
					return th264, i
				}
			}
		}
		return nil, -1
	}()
	if h264TrackID < 0 {
		return nil
	}

	if h264Track.SPS() != nil && h264Track.PPS() != nil {
		return nil
	}

	s.log(logger.Info, "source has not provided H264 parameters (SPS and PPS)"+
		" inside the SDP; extracting them from the stream...")

	var streamMutex sync.RWMutex
	var stream *stream
	decoder := &rtph264.Decoder{}
	decoder.Init()
	var sps []byte
	var pps []byte
	paramsReceived := make(chan struct{})

	c.OnPacketRTP = func(trackID int, pkt *rtp.Packet) {
		streamMutex.RLock()
		defer streamMutex.RUnlock()

		if stream == nil {
			if trackID != h264TrackID {
				return
			}

			select {
			case <-paramsReceived:
				return
			default:
			}

			nalus, _, err := decoder.Decode(pkt)
			if err != nil {
				return
			}

			for _, nalu := range nalus {
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeSPS:
					sps = nalu
					if sps != nil && pps != nil {
						close(paramsReceived)
					}

				case h264.NALUTypePPS:
					pps = nalu
					if sps != nil && pps != nil {
						close(paramsReceived)
					}
				}
			}
		} else {
			stream.writePacketRTP(trackID, pkt)
		}
	}

	_, err := c.Play(nil)
	if err != nil {
		return err
	}

	readErr := make(chan error)
	go func() {
		readErr <- c.Wait()
	}()

	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()

	select {
	case err := <-readErr:
		return err

	case <-timeout.C:
		c.Close()
		<-readErr
		return fmt.Errorf("source did not send H264 parameters in time")

	case <-paramsReceived:
		s.log(logger.Info, "H264 parameters extracted")

		h264Track.SetSPS(sps)
		h264Track.SetPPS(pps)

		res := s.parent.onSourceStaticSetReady(pathSourceStaticSetReadyReq{
			source: s,
			tracks: tracks,
		})
		if res.err != nil {
			c.Close()
			<-readErr
			return res.err
		}

		func() {
			streamMutex.Lock()
			defer streamMutex.Unlock()
			stream = res.stream
		}()

		s.log(logger.Info, "ready")

		defer func() {
			s.parent.onSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{source: s})
		}()

		return <-readErr
	}
}

// onSourceAPIDescribe implements source.
func (*rtspSource) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtspSource"}
}
