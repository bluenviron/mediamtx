// Package rtsp contains the RTSP static source.
package rtsp

import (
	"net/url"
	"regexp"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/headers"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/rtsp"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
)

func createRangeHeader(cnf *conf.Path) (*headers.Range, error) {
	switch cnf.RTSPRangeType {
	case conf.RTSPRangeTypeClock:
		start, err := time.Parse("20060102T150405Z", cnf.RTSPRangeStart)
		if err != nil {
			return nil, err
		}

		return &headers.Range{
			Value: &headers.RangeUTC{
				Start: start,
			},
		}, nil

	case conf.RTSPRangeTypeNPT:
		start, err := time.ParseDuration(cnf.RTSPRangeStart)
		if err != nil {
			return nil, err
		}

		return &headers.Range{
			Value: &headers.RangeNPT{
				Start: start,
			},
		}, nil

	case conf.RTSPRangeTypeSMPTE:
		start, err := time.ParseDuration(cnf.RTSPRangeStart)
		if err != nil {
			return nil, err
		}

		return &headers.Range{
			Value: &headers.RangeSMPTE{
				Start: headers.RangeSMPTETime{
					Time: start,
				},
			},
		}, nil

	default:
		return nil, nil
	}
}

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a RTSP static source.
type Source struct {
	ReadTimeout    conf.Duration
	WriteTimeout   conf.Duration
	WriteQueueSize int
	Parent         parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[RTSP source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	packetsLost := &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Log(logger.Warn, "%d RTP %s lost",
				val,
				func() string {
					if val == 1 {
						return "packet"
					}
					return "packets"
				}())
		},
	}

	packetsLost.Start()
	defer packetsLost.Stop()

	decodeErrors := &counterdumper.CounterDumper{
		OnReport: func(val uint64) {
			s.Log(logger.Warn, "%d decode %s",
				val,
				func() string {
					if val == 1 {
						return "error"
					}
					return "errors"
				}())
		},
	}

	decodeErrors.Start()
	defer decodeErrors.Stop()

	u0, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	var scheme string
	if u0.Scheme == "rtsp" || u0.Scheme == "rtsp+http" || u0.Scheme == "rtsp+ws" {
		scheme = "rtsp"
	} else {
		scheme = "rtsps"
	}

	var tunnel gortsplib.Tunnel
	switch u0.Scheme {
	case "rtsp+http", "rtsps+http":
		tunnel = gortsplib.TunnelHTTP
	case "rtsp+ws", "rtsps+ws":
		tunnel = gortsplib.TunnelWebSocket
	default:
		tunnel = gortsplib.TunnelNone
	}

	u, err := base.ParseURL(regexp.MustCompile("^.*?://").ReplaceAllString(params.ResolvedSource, "rtsp://"))
	if err != nil {
		return err
	}

	c := &gortsplib.Client{
		Scheme:            scheme,
		Host:              u.Host,
		Tunnel:            tunnel,
		Protocol:          params.Conf.RTSPTransport.Protocol,
		TLSConfig:         tls.MakeConfig(u.Hostname(), params.Conf.SourceFingerprint),
		ReadTimeout:       time.Duration(s.ReadTimeout),
		WriteTimeout:      time.Duration(s.WriteTimeout),
		WriteQueueSize:    s.WriteQueueSize,
		UDPReadBufferSize: int(params.Conf.RTSPUDPReadBufferSize),
		AnyPortEnable:     params.Conf.RTSPAnyPort,
		OnRequest: func(req *base.Request) {
			s.Log(logger.Debug, "[c->s] %v", req)
		},
		OnResponse: func(res *base.Response) {
			s.Log(logger.Debug, "[s->c] %v", res)
		},
		OnTransportSwitch: func(err error) {
			s.Log(logger.Warn, err.Error())
		},
		OnPacketsLost: func(lost uint64) {
			packetsLost.Add(lost)
		},
		OnDecodeError: func(_ error) {
			decodeErrors.Increase()
		},
	}

	err = c.Start()
	if err != nil {
		return err
	}
	defer c.Close()

	readErr := make(chan error)
	go func() {
		readErr <- func() error {
			desc, _, err2 := c.Describe(u)
			if err2 != nil {
				return err2
			}

			err2 = c.SetupAll(desc.BaseURL, desc.Medias)
			if err2 != nil {
				return err2
			}

			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:               desc,
				GenerateRTPPackets: false,
				FillNTP:            !params.Conf.UseAbsoluteTimestamp,
			})
			if res.Err != nil {
				return res.Err
			}

			defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

			rtsp.ToStream(
				c,
				desc.Medias,
				params.Conf,
				res.Stream,
				s)

			rangeHeader, err2 := createRangeHeader(params.Conf)
			if err2 != nil {
				return err2
			}

			_, err2 = c.Play(rangeHeader)
			if err2 != nil {
				return err2
			}

			return c.Wait()
		}()
	}()

	for {
		select {
		case err = <-readErr:
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			c.Close()
			<-readErr
			return nil
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "rtspSource",
		ID:   "",
	}
}
