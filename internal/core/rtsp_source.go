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
	OnSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq)
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

	tlsConfig := &tls.Config{}
	if s.fingerprint != "" {
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			h := sha256.New()
			h.Write(cs.PeerCertificates[0].Raw)
			hstr := hex.EncodeToString(h.Sum(nil))
			fingerprintLower := strings.ToLower(s.fingerprint)

			if hstr != fingerprintLower {
				return fmt.Errorf("server fingerprint do not match: expected %s, got %s",
					fingerprintLower, hstr)
			}

			return nil
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

	readErr := make(chan error)
	go func() {
		readErr <- func() error {
			_, err = c.Options(u)
			if err != nil {
				return err
			}

			tracks, baseURL, _, err := c.Describe(u)
			if err != nil {
				return err
			}

			for _, t := range tracks {
				_, err := c.Setup(true, baseURL, t, 0, 0)
				if err != nil {
					return err
				}
			}

			err = s.handleMissingH264Params(c, tracks)
			if err != nil {
				return err
			}

			res := s.parent.onSourceStaticSetReady(pathSourceStaticSetReadyReq{
				Source: s,
				Tracks: c.Tracks(),
			})
			if res.Err != nil {
				return res.Err
			}

			s.log(logger.Info, "ready")

			defer func() {
				s.parent.OnSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{Source: s})
			}()

			c.OnPacketRTP = func(trackID int, payload []byte) {
				res.Stream.onPacketRTP(trackID, payload)
			}

			c.OnPacketRTCP = func(trackID int, payload []byte) {
				res.Stream.onPacketRTCP(trackID, payload)
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
	h264TrackID := func() int {
		for i, t := range tracks {
			if t.IsH264() {
				return i
			}
		}
		return -1
	}()
	if h264TrackID < 0 {
		return nil
	}

	_, err := tracks[h264TrackID].ExtractConfigH264()
	if err == nil {
		return nil
	}

	s.log(logger.Info, "source has not provided H264 parameters (SPS and PPS)"+
		" inside the SDP; extracting them from the stream...")

	var streamMutex sync.RWMutex
	var stream *stream
	var payloadType uint8
	decoder := rtph264.NewDecoder()
	var sps []byte
	var pps []byte
	paramsReceived := make(chan struct{})

	c.OnPacketRTP = func(trackID int, payload []byte) {
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

			var pkt rtp.Packet
			err := pkt.Unmarshal(payload)
			if err != nil {
				return
			}

			nalus, _, err := decoder.Decode(&pkt)
			if err != nil {
				return
			}

			payloadType = pkt.Header.PayloadType

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
			stream.onPacketRTP(trackID, payload)
		}
	}

	c.OnPacketRTCP = func(trackID int, payload []byte) {
		streamMutex.RLock()
		defer streamMutex.RUnlock()

		if stream != nil {
			stream.onPacketRTCP(trackID, payload)
		}
	}

	_, err = c.Play(nil)
	if err != nil {
		return err
	}

	waitError := make(chan error)
	go func() {
		waitError <- c.Wait()
	}()

	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()

	select {
	case err := <-waitError:
		return err

	case <-timeout.C:
		return fmt.Errorf("source did not send H264 parameters in time")

	case <-paramsReceived:
		s.log(logger.Info, "H264 parameters extracted")

		track, err := gortsplib.NewTrackH264(payloadType, &gortsplib.TrackConfigH264{
			SPS: sps,
			PPS: pps,
		})
		if err != nil {
			return err
		}

		tracks[h264TrackID] = track

		res := s.parent.onSourceStaticSetReady(pathSourceStaticSetReadyReq{
			Source: s,
			Tracks: tracks,
		})
		if res.Err != nil {
			return res.Err
		}

		func() {
			streamMutex.Lock()
			defer streamMutex.Unlock()
			stream = res.Stream
		}()

		s.log(logger.Info, "ready")

		defer func() {
			s.parent.OnSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{Source: s})
		}()
	}

	return <-waitError
}

// onSourceAPIDescribe implements source.
func (*rtspSource) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtspSource"}
}
