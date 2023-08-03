package core

import (
	"context"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/base"
	"github.com/bluenviron/gortsplib/v3/pkg/headers"
	"github.com/pion/rtp"

	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func createRangeHeader(cnf *conf.PathConf) (*headers.Range, error) {
	switch cnf.RtspRangeType {
	case conf.RtspRangeTypeClock:
		start, err := time.Parse("20060102T150405Z", cnf.RtspRangeStart)
		if err != nil {
			return nil, err
		}

		return &headers.Range{
			Value: &headers.RangeUTC{
				Start: start,
			},
		}, nil

	case conf.RtspRangeTypeNPT:
		start, err := time.ParseDuration(cnf.RtspRangeStart)
		if err != nil {
			return nil, err
		}

		return &headers.Range{
			Value: &headers.RangeNPT{
				Start: start,
			},
		}, nil

	case conf.RtspRangeTypeSMPTE:
		start, err := time.ParseDuration(cnf.RtspRangeStart)
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

type rtspSourceParent interface {
	logger.Writer
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
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
	s.parent.Log(level, "[RTSP source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *rtspSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	s.Log(logger.Debug, "connecting")

	c := &gortsplib.Client{
		Transport:       cnf.SourceProtocol.Transport,
		TLSConfig:       tlsConfigForFingerprint(cnf.SourceFingerprint),
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

			res := s.parent.setReady(pathSourceStaticSetReadyReq{
				medias:             medias,
				generateRTPPackets: false,
			})
			if res.err != nil {
				return res.err
			}

			defer s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})

			for _, medi := range medias {
				for _, forma := range medi.Formats {
					cmedi := medi
					cforma := forma

					c.OnPacketRTP(cmedi, cforma, func(pkt *rtp.Packet) {
						res.stream.WriteRTPPacket(cmedi, cforma, pkt, time.Now())
					})
				}
			}

			rangeHeader, err := createRangeHeader(cnf)
			if err != nil {
				return err
			}

			_, err = c.Play(rangeHeader)
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
func (*rtspSource) apiSourceDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "rtspSource",
		ID:   "",
	}
}
