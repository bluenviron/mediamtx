package core

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/base"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/pion/rtp"

	"github.com/aler9/gortsplib/v2/pkg/url"
	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
	"github.com/aler9/rtsp-simple-server/internal/logger"
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
		Log: func(level gortsplib.LogLevel, format string, args ...interface{}) {
			s.Log(logger.Warn, format, args...)
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
					cmedia := medi
					cformat := forma

					switch forma.(type) {
					case *format.H264:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.UnitH264{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.H265:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.UnitH265{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.VP8:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.UnitVP8{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.VP9:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.UnitVP9{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.MPEG4Audio:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.UnitMPEG4Audio{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.Opus:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.UnitOpus{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					default:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.UnitGeneric{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})
					}
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
