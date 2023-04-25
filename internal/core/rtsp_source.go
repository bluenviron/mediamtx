package core

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/base"
	"github.com/pion/rtp"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/logger"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
)

type rtspSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type rtspSource struct {
	readTimeout     conf.StringDuration
	writeTimeout    conf.StringDuration
	readBufferCount int
	parent          rtspSourceParent
}

func newRTSPSource(
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	readBufferCount int,
	parent rtspSourceParent,
) *rtspSource {
	return &rtspSource{
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
func (s *rtspSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	s.Log(logger.Debug, "connecting")

	var tlsConfig *tls.Config
	if cnf.SourceFingerprint != "" {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				h := sha256.New()
				h.Write(cs.PeerCertificates[0].Raw)
				hstr := hex.EncodeToString(h.Sum(nil))
				fingerprintLower := strings.ToLower(cnf.SourceFingerprint)

				if hstr != fingerprintLower {
					return fmt.Errorf("server fingerprint do not match: expected %s, got %s",
						fingerprintLower, hstr)
				}

				return nil
			},
		}
	}

	c := &gortsplib.Client{
		Transport:       cnf.SourceProtocol.Transport,
		TLSConfig:       tlsConfig,
		ReadTimeout:     time.Duration(s.readTimeout),
		WriteTimeout:    time.Duration(s.writeTimeout),
		ReadBufferCount: s.readBufferCount,
		AnyPortEnable:   cnf.SourceAnyPortEnable,
		OnRequest: func(req *base.Request) {
			s.Log(logger.Debug, "c->s %v", req)
		},
		OnResponse: func(res *base.Response) {
			s.Log(logger.Debug, "s->c %v", res)
		},
		OnTransportSwitch: func(err error) {
			s.Log(logger.Warn, err.Error())
		},
		OnPacketLost: func(err error) {
			s.Log(logger.Warn, err.Error())
		},
		OnDecodeError: func(err error) {
			s.Log(logger.Warn, err.Error())
		},
	}

	u, err := url.Parse(cnf.Source)
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
			medias, baseURL, _, err := c.Describe(u)
			if err != nil {
				return err
			}

			err = c.SetupAll(medias, baseURL)
			if err != nil {
				return err
			}

			res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
				medias:             medias,
				generateRTPPackets: false,
			})
			if res.err != nil {
				return res.err
			}

			s.Log(logger.Info, "ready: %s", sourceMediaInfo(medias))

			defer func() {
				s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
			}()

			for _, medi := range medias {
				for _, forma := range medi.Formats {
					writeFunc := getRTSPWriteFunc(medi, forma, res.stream)

					c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
						err := writeFunc(pkt)
						if err != nil {
							s.Log(logger.Warn, "%v", err)
						}
					})
				}
			}

			_, err = c.Play(nil)
			if err != nil {
				return err
			}

			return c.Wait()
		}()
	}()

	for {
		select {
		case err := <-readErr:
			return err

		case <-reloadConf:

		case <-ctx.Done():
			c.Close()
			<-readErr
			return nil
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*rtspSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtspSource"}
}
