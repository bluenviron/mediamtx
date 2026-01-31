// Package rtp contains the RTP static source.
package rtp

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/rtptime"
	"github.com/bluenviron/gortsplib/v5/pkg/sdp"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/counterdumper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/errordumper"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/udp"
	"github.com/bluenviron/mediamtx/internal/protocols/unix"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
)

type parent interface {
	logger.Writer
	SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes
	SetNotReady(req defs.PathSourceStaticSetNotReadyReq)
}

// Source is a RTP static source.
type Source struct {
	ReadTimeout       conf.Duration
	UDPReadBufferSize uint
	Parent            parent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...any) {
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
		udpReadBufferSize := s.UDPReadBufferSize
		if params.Conf.RTPUDPReadBufferSize != nil {
			udpReadBufferSize = *params.Conf.RTPUDPReadBufferSize
		}

		nc, err = udp.CreateConn(u, int(udpReadBufferSize))
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

	var subStream *stream.SubStream

	timeDecoder := &rtptime.GlobalDecoder{}
	timeDecoder.Initialize()

	mediasByPayloadType := make(map[uint8]*rtpMedia)
	formatsByPayloadType := make(map[uint8]*rtpFormat)

	for _, descMedia := range desc.Medias {
		rtpMedia := &rtpMedia{
			desc: descMedia,
		}

		for _, descFormat := range descMedia.Formats {
			rtpFormat := &rtpFormat{
				desc: descFormat,
			}
			rtpFormat.initialize()

			mediasByPayloadType[descFormat.PayloadType()] = rtpMedia
			formatsByPayloadType[descFormat.PayloadType()] = rtpFormat
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
			if subStream != nil {
				decodeErrors.Add(err)
				continue
			}
			return err
		}

		if subStream == nil {
			res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
				Desc:          desc,
				UseRTPPackets: true,
				ReplaceNTP:    true,
			})
			if res.Err != nil {
				return res.Err
			}

			defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

			subStream = res.SubStream
		}

		media, ok := mediasByPayloadType[pkt.PayloadType]
		if !ok {
			continue
		}

		forma := formatsByPayloadType[pkt.PayloadType]

		pkts, lost := forma.rtpReceiver.ProcessPacket2(&pkt, time.Now(), forma.desc.PTSEqualsDTS(&pkt))

		if lost != 0 {
			packetsLost.Add(lost)
		}

		for _, pkt := range pkts {
			pts, ok2 := timeDecoder.Decode(forma.desc, pkt)
			if !ok2 {
				continue
			}

			subStream.WriteUnit(media.desc, forma.desc, &unit.Unit{
				PTS:        pts,
				RTPPackets: []*rtp.Packet{pkt},
			})
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() *defs.APIPathSource {
	return &defs.APIPathSource{
		Type: "rtpSource",
		ID:   "",
	}
}
