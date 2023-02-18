package core

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/codecs/mpeg4audio"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/asticode/go-astits"
	"golang.org/x/net/ipv4"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/formatprocessor"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/mpegts"
)

const (
	multicastTTL = 16
)

type readerFunc func([]byte) (int, error)

func (rf readerFunc) Read(p []byte) (int, error) {
	return rf(p)
}

type udpSourceParent interface {
	log(logger.Level, string, ...interface{})
	sourceStaticImplSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	sourceStaticImplSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type udpSource struct {
	readTimeout conf.StringDuration
	parent      udpSourceParent
}

func newUDPSource(
	readTimeout conf.StringDuration,
	parent udpSourceParent,
) *udpSource {
	return &udpSource{
		readTimeout: readTimeout,
		parent:      parent,
	}
}

func (s *udpSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[udp source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *udpSource) run(ctx context.Context, cnf *conf.PathConf, reloadConf chan *conf.PathConf) error {
	s.Log(logger.Debug, "connecting")

	hostPort := cnf.Source[len("udp://"):]

	pc, err := net.ListenPacket("udp", hostPort)
	if err != nil {
		return err
	}
	defer pc.Close()

	host, _, _ := net.SplitHostPort(hostPort)
	ip := net.ParseIP(host)

	if ip.IsMulticast() {
		p := ipv4.NewPacketConn(pc)

		err = p.SetMulticastTTL(multicastTTL)
		if err != nil {
			return err
		}

		intfs, err := net.Interfaces()
		if err != nil {
			return err
		}

		for _, intf := range intfs {
			err := p.JoinGroup(&intf, &net.UDPAddr{IP: ip})
			if err != nil {
				return err
			}
		}
	}

	midbuffer := make([]byte, 0, 1472) // UDP MTU
	midbufferPos := 0

	readPacket := func(buf []byte) (int, error) {
		if midbufferPos < len(midbuffer) {
			n := copy(buf, midbuffer[midbufferPos:])
			midbufferPos += n
			return n, nil
		}

		mn, _, err := pc.ReadFrom(midbuffer[:cap(midbuffer)])
		if err != nil {
			return 0, err
		}

		if (mn % 188) != 0 {
			return 0, fmt.Errorf("received packet with size %d not multiple of 188", mn)
		}

		midbuffer = midbuffer[:mn]
		n := copy(buf, midbuffer)
		midbufferPos = n
		return n, nil
	}

	dem := astits.NewDemuxer(
		context.Background(),
		readerFunc(readPacket),
		astits.DemuxerOptPacketSize(188))

	readerErr := make(chan error)

	go func() {
		readerErr <- func() error {
			pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
			tracks, err := mpegts.FindTracks(dem)
			if err != nil {
				return err
			}

			var medias media.Medias
			mediaCallbacks := make(map[uint16]func(time.Duration, []byte), len(tracks))
			var stream *stream

			for _, track := range tracks {
				medi := &media.Media{
					Formats: []format.Format{track.Format},
				}
				medias = append(medias, medi)
				cformat := track.Format

				switch track.Format.(type) {
				case *format.H264:
					medi.Type = media.TypeVideo

					mediaCallbacks[track.ES.ElementaryPID] = func(pts time.Duration, data []byte) {
						au, err := h264.AnnexBUnmarshal(data)
						if err != nil {
							s.Log(logger.Warn, "%v", err)
							return
						}

						err = stream.writeData(medi, cformat, &formatprocessor.DataH264{
							PTS: pts,
							AU:  au,
							NTP: time.Now(),
						})
						if err != nil {
							s.Log(logger.Warn, "%v", err)
						}
					}

				case *format.MPEG4Audio:
					medi.Type = media.TypeAudio

					mediaCallbacks[track.ES.ElementaryPID] = func(pts time.Duration, data []byte) {
						var pkts mpeg4audio.ADTSPackets
						err := pkts.Unmarshal(data)
						if err != nil {
							s.Log(logger.Warn, "%v", err)
							return
						}

						aus := make([][]byte, len(pkts))
						for i, pkt := range pkts {
							aus[i] = pkt.AU
						}

						err = stream.writeData(medi, cformat, &formatprocessor.DataMPEG4Audio{
							PTS: pts,
							AUs: aus,
							NTP: time.Now(),
						})
						if err != nil {
							s.Log(logger.Warn, "%v", err)
						}
					}
				}
			}

			res := s.parent.sourceStaticImplSetReady(pathSourceStaticSetReadyReq{
				medias:             medias,
				generateRTPPackets: true,
			})
			if res.err != nil {
				return res.err
			}

			defer func() {
				s.parent.sourceStaticImplSetNotReady(pathSourceStaticSetNotReadyReq{})
			}()

			s.Log(logger.Info, "ready: %s", sourceMediaInfo(medias))

			stream = res.stream
			var timedec *mpegts.TimeDecoder

			for {
				pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
				data, err := dem.NextData()
				if err != nil {
					return err
				}

				if data.PES == nil {
					continue
				}

				if data.PES.Header.OptionalHeader == nil ||
					data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorNoPTSOrDTS ||
					data.PES.Header.OptionalHeader.PTSDTSIndicator == astits.PTSDTSIndicatorIsForbidden {
					return fmt.Errorf("PTS is missing")
				}

				var pts time.Duration

				if timedec == nil {
					timedec = mpegts.NewTimeDecoder(data.PES.Header.OptionalHeader.PTS.Base)
					pts = 0
				} else {
					pts = timedec.Decode(data.PES.Header.OptionalHeader.PTS.Base)
				}

				cb, ok := mediaCallbacks[data.PID]
				if !ok {
					continue
				}

				cb(pts, data.PES.Data)
			}
		}()
	}()

	select {
	case err := <-readerErr:
		return err

	case <-ctx.Done():
		pc.Close()
		<-readerErr
		return fmt.Errorf("terminated")
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*udpSource) apiSourceDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"udpSource"}
}
