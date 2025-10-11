// Package rtp contains the RTP static source.
package rtp

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/rtptime"
	"github.com/bluenviron/gortsplib/v5/pkg/sdp"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/udp"
	"github.com/bluenviron/mediamtx/internal/protocols/unix"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/pion/rtp"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a RTP static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[RTP source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	var sd sdp.SessionDescription
	err := sd.Unmarshal([]byte(params.Conf.RTPSDP))
	if err != nil {
		return err
	}

	var desc description.Session
	err = desc.Unmarshal(&sd)
	if err != nil {
		return err
	}

	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(params.ResolvedSource)
	if err != nil {
		return err
	}

	var nc net.Conn

	switch u.Scheme {
	case "unix+rtp":
		nc, err = unix.CreateConn(u)
		if err != nil {
			return err
		}

	default:
		nc, err = udp.CreateConn(u, int(params.Conf.RTPUDPReadBufferSize))
		if err != nil {
			return err
		}
	}

	readerErr := make(chan error)
	go func() {
		readerErr <- s.runReader(&desc, nc)
	}()

	for {
		select {
		case err = <-readerErr:
			nc.Close()
			return err

		case <-params.ReloadConf:

		case <-params.Context.Done():
			nc.Close()
			<-readerErr
			return fmt.Errorf("terminated")
		}
	}
}

func (s *Source) runReader(desc *description.Session, nc net.Conn) error {
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

	var stream *stream.Stream

	timeDecoder := &rtptime.GlobalDecoder{}
	timeDecoder.Initialize()

	mediasByPayloadType := make(map[uint8]*description.Media)
	formatsByPayloadType := make(map[uint8]format.Format)

	for _, media := range desc.Medias {
		for _, forma := range media.Formats {
			mediasByPayloadType[forma.PayloadType()] = media
			formatsByPayloadType[forma.PayloadType()] = forma
		}
	}

	for {
		buf := make([]byte, 1500)
		nc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		n, err := nc.Read(buf)
		if err != nil {
			return err
		}

		var pkt rtp.Packet
		err = pkt.Unmarshal(buf[:n])
		if err != nil {
			if stream != nil {
				decodeErrors.Increase()
				continue
			}
			return err
		}

		if stream == nil {
			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:               desc,
				GenerateRTPPackets: false,
				FillNTP:            true,
			})
			if res.Err != nil {
				return res.Err
			}

			defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

			stream = res.Stream
		}

		media, ok := mediasByPayloadType[pkt.PayloadType]
		if !ok {
			continue
		}

		forma := formatsByPayloadType[pkt.PayloadType]

		pts, ok := timeDecoder.Decode(forma, &pkt)
		if !ok {
			continue
		}

		stream.WriteRTPPacket(media, forma, &pkt, time.Time{}, pts)
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "rtpSource",
		ID:   "",
	}
}
