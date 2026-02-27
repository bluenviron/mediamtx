// Package rtsp contains the RTSP static source.
package rtsp

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/headers"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/packetdumper"
	"github.com/bluenviron/mediamtx/internal/protocols/rtsp"
	"github.com/bluenviron/mediamtx/internal/protocols/tls"
	"github.com/bluenviron/mediamtx/internal/stream"
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
	DumpPackets       bool
	ReadTimeout       conf.Duration
	WriteTimeout      conf.Duration
	WriteQueueSize    int
	UDPReadBufferSize uint
	Parent            parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[RTSP source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	packetsLost := &counterdumper.Dumper{
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

	decodeErrors := &errordumper.Dumper{
		OnReport: func(val uint64, last error) {
			if val == 1 {
				s.Log(logger.Warn, "decode error: %v", last)
			} else {
				s.Log(logger.Warn, "%d decode errors, last was: %v", val, last)
			}
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

	u, err := base.ParseURL(regexp.MustCompile("^.*?://").ReplaceAllString(params.ResolvedSource, "rtsp://"))
	if err != nil {
		return err
	}

	c := &gortsplib.Client{
		Scheme:            scheme,
		Host:              u.Host,
		Protocol:          params.Conf.RTSPTransport.Protocol,
		TLSConfig:         tls.MakeConfig(u.Hostname(), params.Conf.SourceFingerprint),
		ReadTimeout:       time.Duration(s.ReadTimeout),
		WriteTimeout:      time.Duration(s.WriteTimeout),
		UDPReadBufferSize: int(s.UDPReadBufferSize),
		WriteQueueSize:    s.WriteQueueSize,
		AnyPortEnable:     params.Conf.RTSPAnyPort,
		UDPSourcePortRange: [2]uint16{
			uint16(params.Conf.RTSPUDPSourcePortRange[0]),
			uint16(params.Conf.RTSPUDPSourcePortRange[1]),
		},
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
		OnDecodeError: func(err error) {
			decodeErrors.Add(err)
		},
	}

	switch u0.Scheme {
	case "rtsp+http", "rtsps+http":
		c.Tunnel = gortsplib.TunnelHTTP
	case "rtsp+ws", "rtsps+ws":
		c.Tunnel = gortsplib.TunnelWebSocket
	}

	if params.Conf.RTSPUDPReadBufferSize != nil {
		s.UDPReadBufferSize = *params.Conf.RTSPUDPReadBufferSize
	}

	if s.DumpPackets {
		c.DialContext = (&packetdumper.DialContext{
			Prefix:      "rtsp_source_conn",
			DialContext: (&net.Dialer{}).DialContext,
		}).Do

		c.ListenPacket = (&packetdumper.ListenPacket{
			Prefix:       "rtsp_source_packetconn",
			ListenPacket: net.ListenPacket,
		}).Do
	}

	err = c.Start()
	if err != nil {
		return err
	}
	defer c.Close()

	readErr := make(chan error)
	go func() {
		readErr <- s.runInner(c, u, params.Conf)
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

func (s *Source) runInner(c *gortsplib.Client, u *base.URL, pathConf *conf.Path) error {
	desc, _, err := c.Describe(u)
	if err != nil {
		return err
	}

	var medias []*description.Media

	for _, m := range desc.Medias {
		if !m.IsBackChannel {
			_, err = c.Setup(desc.BaseURL, m, 0, 0)
			if err != nil {
				return err
			}

			medias = append(medias, m)
		}
	}

	if medias == nil {
		return fmt.Errorf("no medias have been setupped")
	}

	desc2 := &description.Session{
		Title:  desc.Title,
		Medias: medias,
	}

	var subStream *stream.SubStream

	rtsp.ToStream(
		c,
		desc2.Medias,
		pathConf,
		&subStream,
		s)

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:          desc2,
		UseRTPPackets: true,
		ReplaceNTP:    !pathConf.UseAbsoluteTimestamp,
	})
	if res.Err != nil {
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	subStream = res.SubStream

	rangeHeader, err := createRangeHeader(pathConf)
	if err != nil {
		return err
	}

	_, err = c.Play(rangeHeader)
	if err != nil {
		return err
	}

	return c.Wait()
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: "rtspSource",
		ID:   "",
	}
}
