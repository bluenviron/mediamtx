package core

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"golang.org/x/net/ipv4"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	multicastTTL = 16
	udpMTU       = 1472
)

func joinMulticastGroupOnAtLeastOneInterface(p *ipv4.PacketConn, listenIP net.IP) error {
	intfs, err := net.Interfaces()
	if err != nil {
		return err
	}

	success := false

	for _, intf := range intfs {
		if (intf.Flags & net.FlagMulticast) != 0 {
			err := p.JoinGroup(&intf, &net.UDPAddr{IP: listenIP})
			if err == nil {
				success = true
			}
		}
	}

	if !success {
		return fmt.Errorf("unable to activate multicast on any network interface")
	}

	return nil
}

type packetConnReader struct {
	net.PacketConn
}

func newPacketConnReader(pc net.PacketConn) *packetConnReader {
	return &packetConnReader{
		PacketConn: pc,
	}
}

func (r *packetConnReader) Read(p []byte) (int, error) {
	n, _, err := r.PacketConn.ReadFrom(p)
	return n, err
}

type udpSourceParent interface {
	logger.Writer
	setReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	setNotReady(req pathSourceStaticSetNotReadyReq)
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
	s.parent.Log(level, "[UDP source] "+format, args...)
}

// run implements sourceStaticImpl.
func (s *udpSource) run(ctx context.Context, cnf *conf.PathConf, _ chan *conf.PathConf) error {
	s.Log(logger.Debug, "connecting")

	hostPort := cnf.Source[len("udp://"):]

	addr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return err
	}

	pc, err := net.ListenPacket(restrictNetwork("udp", addr.String()))
	if err != nil {
		return err
	}
	defer pc.Close()

	if addr.IP.IsMulticast() {
		p := ipv4.NewPacketConn(pc)

		err = p.SetMulticastTTL(multicastTTL)
		if err != nil {
			return err
		}

		err = joinMulticastGroupOnAtLeastOneInterface(p, addr.IP)
		if err != nil {
			return err
		}
	}

	readerErr := make(chan error)
	go func() {
		readerErr <- s.runReader(pc)
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

func (s *udpSource) runReader(pc net.PacketConn) error {
	pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
	r, err := mpegts.NewReader(mpegts.NewBufferedReader(newPacketConnReader(pc)))
	if err != nil {
		return err
	}

	var medias media.Medias
	var stream *stream.Stream

	var td *mpegts.TimeDecoder
	decodeTime := func(t int64) time.Duration {
		if td == nil {
			td = mpegts.NewTimeDecoder(t)
		}
		return td.Decode(t)
	}

	for _, track := range r.Tracks() { //nolint:dupl
		var medi *media.Media

		switch tcodec := track.Codec.(type) {
		case *mpegts.CodecH264:
			medi = &media.Media{
				Type: media.TypeVideo,
				Formats: []formats.Format{&formats.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitH264{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: decodeTime(pts),
					AU:  au,
				})
				return nil
			})

		case *mpegts.CodecH265:
			medi = &media.Media{
				Type: media.TypeVideo,
				Formats: []formats.Format{&formats.H265{
					PayloadTyp: 96,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitH265{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: decodeTime(pts),
					AU:  au,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Audio:
			medi = &media.Media{
				Type: media.TypeAudio,
				Formats: []formats.Format{&formats.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config:           &tcodec.Config,
				}},
			}

			r.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitMPEG4AudioGeneric{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS: decodeTime(pts),
					AUs: aus,
				})
				return nil
			})

		case *mpegts.CodecOpus:
			medi = &media.Media{
				Type: media.TypeAudio,
				Formats: []formats.Format{&formats.Opus{
					PayloadTyp: 96,
					IsStereo:   (tcodec.ChannelCount == 2),
				}},
			}

			r.OnDataOpus(track, func(pts int64, packets [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitOpus{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS:     decodeTime(pts),
					Packets: packets,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Audio:
			medi = &media.Media{
				Type:    media.TypeAudio,
				Formats: []formats.Format{&formats.MPEG1Audio{}},
			}

			r.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &formatprocessor.UnitMPEG1Audio{
					BaseUnit: formatprocessor.BaseUnit{
						NTP: time.Now(),
					},
					PTS:    decodeTime(pts),
					Frames: frames,
				})
				return nil
			})

		default:
			continue
		}

		medias = append(medias, medi)
	}

	if len(medias) == 0 {
		return fmt.Errorf("no supported tracks found")
	}

	res := s.parent.setReady(pathSourceStaticSetReadyReq{
		medias:             medias,
		generateRTPPackets: true,
	})
	if res.err != nil {
		return res.err
	}

	defer s.parent.setNotReady(pathSourceStaticSetNotReadyReq{})

	stream = res.stream

	for {
		pc.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

// apiSourceDescribe implements sourceStaticImpl.
func (*udpSource) apiSourceDescribe() pathAPISourceOrReader {
	return pathAPISourceOrReader{
		Type: "udpSource",
		ID:   "",
	}
}
