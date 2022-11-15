package core

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/pion/rtp"

	"github.com/aler9/gortsplib/pkg/url"
	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type rtspSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type rtspSource struct {
	ur              string
	proto           conf.SourceProtocol
	anyPortEnable   bool
	fingerprint     string
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	parent          rtspSourceParent
}

func newRTSPSource(
	ur string,
	proto conf.SourceProtocol,
	anyPortEnable bool,
	fingerprint string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	parent rtspSourceParent,
) *rtspSource {
	return &rtspSource{
		ur:              ur,
		proto:           proto,
		anyPortEnable:   anyPortEnable,
		fingerprint:     fingerprint,
		readTimeout:     readTimeout,
		writeTimeout:    writeTimeout,
		readBufferCount: readBufferCount,
		parent:          parent,
	}
}

func (s *rtspSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[rtsp source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *rtspSource) run(ctx context.Context) error {
	s.Log(logger.Debug, "connecting")

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
		AnyPortEnable:   s.anyPortEnable,
		OnRequest: func(req *base.Request) {
			s.Log(logger.Debug, "c->s %v", req)
		},
		OnResponse: func(res *base.Response) {
			s.Log(logger.Debug, "s->c %v", res)
		},
		OnDecodeError: func(err error) {
			s.Log(logger.Warn, "%v", err)
		},
	}

	u, err := url.Parse(s.ur)
	if err != nil {
		return err
	}

	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		return err
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
				_, err := c.Setup(t, baseURL, 0, 0)
				if err != nil {
					return err
				}
			}

			res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
				tracks:             tracks,
				generateRTPPackets: false,
			})
			if res.err != nil {
				return res.err
			}

			s.Log(logger.Info, "ready: %s", sourceTrackInfo(tracks))

			defer func() {
				s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
			}()

			c.OnPacketRTP = func(ctx *gortsplib.ClientOnPacketRTPCtx) {
				var err error

				switch tracks[ctx.TrackID].(type) {
				case *gortsplib.TrackH264:
					err = res.stream.writeData(&dataH264{
						trackID:    ctx.TrackID,
						rtpPackets: []*rtp.Packet{ctx.Packet},
					})

				case *gortsplib.TrackMPEG4Audio:
					err = res.stream.writeData(&dataMPEG4Audio{
						trackID:    ctx.TrackID,
						rtpPackets: []*rtp.Packet{ctx.Packet},
					})

				default:
					err = res.stream.writeData(&dataGeneric{
						trackID:    ctx.TrackID,
						rtpPackets: []*rtp.Packet{ctx.Packet},
					})
				}

				if err != nil {
					s.Log(logger.Warn, "%v", err)
				}
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
		return err

	case <-ctx.Done():
		c.Close()
		<-readErr
		return nil
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*rtspSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtspSource"}
}
