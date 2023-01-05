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
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.DataH264{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.H265:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.DataH265{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.VP8:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.DataVP8{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.VP9:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.DataVP9{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.MPEG4Audio:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.DataMPEG4Audio{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					case *format.Opus:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.DataOpus{
								RTPPackets: []*rtp.Packet{pkt},
								NTP:        time.Now(),
							})
							if err != nil {
								s.Log(logger.Warn, "%v", err)
							}
						})

					default:
						c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
							err := res.stream.writeData(cmedia, cformat, &formatprocessor.DataGeneric{
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
