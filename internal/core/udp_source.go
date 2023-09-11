package core

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/multicast"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	// same size as GStreamer's rtspsrc
	udpKernelReadBufferSize = 0x80000
)

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

type packetConn interface {
	net.PacketConn
	SetReadBuffer(int) error
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

	var pc packetConn

	if ip4 := addr.IP.To4(); ip4 != nil && addr.IP.IsMulticast() {
		pc, err = multicast.NewMultiConn(hostPort, net.ListenPacket)
		if err != nil {
			return err
		}
	} else {
		tmp, err := net.ListenPacket(restrictNetwork("udp", addr.String()))
		if err != nil {
			return err
		}
		pc = tmp.(*net.UDPConn)
	}

	defer pc.Close()

	err = pc.SetReadBuffer(udpKernelReadBufferSize)
	if err != nil {
		return err
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

	decodeErrLogger := logger.NewLimitedLogger(s)

	r.OnDecodeError(func(err error) {
		decodeErrLogger.Log(logger.Warn, err.Error())
	})

	var medias []*description.Media //nolint:prealloc
	var stream *stream.Stream

	var td *mpegts.TimeDecoder
	decodeTime := func(t int64) time.Duration {
		if td == nil {
			td = mpegts.NewTimeDecoder(t)
		}
		return td.Decode(t)
	}

	for _, track := range r.Tracks() { //nolint:dupl
		var medi *description.Media

		switch tcodec := track.Codec.(type) {
		case *mpegts.CodecH265:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecH264:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			r.OnDataH26x(track, func(pts int64, _ int64, au [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecOpus:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp: 96,
					IsStereo:   (tcodec.ChannelCount == 2),
				}},
			}

			r.OnDataOpus(track, func(pts int64, packets [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &unit.Opus{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					Packets: packets,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Audio:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config:           &tcodec.Config,
				}},
			}

			r.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &unit.MPEG4AudioGeneric{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
					AUs: aus,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Audio:
			medi = &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG1Audio{}},
			}

			r.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
				stream.WriteUnit(medi, medi.Formats[0], &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: decodeTime(pts),
					},
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
		desc:               &description.Session{Medias: medias},
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
