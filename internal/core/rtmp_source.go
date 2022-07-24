package core

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/format/flv/flvio"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
	"github.com/aler9/rtsp-simple-server/internal/rtmp/message"
)

const (
	rtmpSourceRetryPause = 5 * time.Second
)

type rtmpSourceParent interface {
	log(logger.Level, string, ...interface{})
	onSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	onSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq)
}

type rtmpSource struct {
	ur           string
	readTimeout  conf.StringDuration
	writeTimeout conf.StringDuration
	wg           *sync.WaitGroup
	parent       rtmpSourceParent

	ctx       context.Context
	ctxCancel func()
}

func newRTMPSource(
	parentCtx context.Context,
	ur string,
	readTimeout conf.StringDuration,
	writeTimeout conf.StringDuration,
	wg *sync.WaitGroup,
	parent rtmpSourceParent,
) *rtmpSource {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtmpSource{
		ur:           ur,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		wg:           wg,
		parent:       parent,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
	}

	s.Log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()

	return s
}

// Close closes a Source.
func (s *rtmpSource) close() {
	s.Log(logger.Info, "stopped")
	s.ctxCancel()
}

func (s *rtmpSource) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[rtmp source] "+format, args...)
}

func (s *rtmpSource) run() {
	defer s.wg.Done()

outer:
	for {
		innerCtx, innerCtxCancel := context.WithCancel(context.Background())
		innerErr := make(chan error)
		go func() {
			innerErr <- s.runInner(innerCtx)
		}()

		select {
		case err := <-innerErr:
			innerCtxCancel()
			s.Log(logger.Info, "ERR: %v", err)

		case <-s.ctx.Done():
			innerCtxCancel()
			<-innerErr
		}

		select {
		case <-time.After(rtmpSourceRetryPause):
		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()
}

func (s *rtmpSource) runInner(innerCtx context.Context) error {
	s.Log(logger.Debug, "connecting")

	u, err := url.Parse(s.ur)
	if err != nil {
		return err
	}

	// add default port
	_, _, err = net.SplitHostPort(u.Host)
	if err != nil {
		u.Host = net.JoinHostPort(u.Host, "1935")
	}

	ctx2, cancel2 := context.WithTimeout(innerCtx, time.Duration(s.readTimeout))
	defer cancel2()

	var d net.Dialer
	nconn, err := d.DialContext(ctx2, "tcp", u.Host)
	if err != nil {
		return err
	}

	conn := rtmp.NewConn(nconn)

	readDone := make(chan error)
	go func() {
		readDone <- func() error {
			nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
			nconn.SetWriteDeadline(time.Now().Add(time.Duration(s.writeTimeout)))
			err = conn.InitializeClient(u, true)
			if err != nil {
				return err
			}

			nconn.SetWriteDeadline(time.Time{})
			nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
			videoTrack, audioTrack, err := conn.ReadTracks()
			if err != nil {
				return err
			}

			var tracks gortsplib.Tracks
			videoTrackID := -1
			audioTrackID := -1

			var h264Encoder *rtph264.Encoder
			if videoTrack != nil {
				h264Encoder = &rtph264.Encoder{PayloadType: 96}
				h264Encoder.Init()
				videoTrackID = len(tracks)
				tracks = append(tracks, videoTrack)
			}

			var aacEncoder *rtpaac.Encoder
			if audioTrack != nil {
				aacEncoder = &rtpaac.Encoder{
					PayloadType:      96,
					SampleRate:       audioTrack.ClockRate(),
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				}
				aacEncoder.Init()
				audioTrackID = len(tracks)
				tracks = append(tracks, audioTrack)
			}

			res := s.parent.onSourceStaticSetReady(pathSourceStaticSetReadyReq{
				source: s,
				tracks: tracks,
			})
			if res.err != nil {
				return res.err
			}

			s.Log(logger.Info, "ready")

			defer func() {
				s.parent.onSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{source: s})
			}()

			for {
				nconn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
				msg, err := conn.ReadMessage()
				if err != nil {
					return err
				}

				switch tmsg := msg.(type) {
				case *message.MsgVideo:
					if tmsg.H264Type == flvio.AVC_NALU {
						if videoTrack == nil {
							return fmt.Errorf("received an H264 packet, but track is not set up")
						}

						nalus, err := h264.AVCCUnmarshal(tmsg.Payload)
						if err != nil {
							return fmt.Errorf("unable to decode AVCC: %v", err)
						}

						pts := tmsg.DTS + tmsg.PTSDelta

						pkts, err := h264Encoder.Encode(nalus, pts)
						if err != nil {
							return fmt.Errorf("error while encoding H264: %v", err)
						}

						lastPkt := len(pkts) - 1
						for i, pkt := range pkts {
							if i != lastPkt {
								res.stream.writeData(&data{
									trackID:      videoTrackID,
									rtp:          pkt,
									ptsEqualsDTS: false,
								})
							} else {
								res.stream.writeData(&data{
									trackID:      videoTrackID,
									rtp:          pkt,
									ptsEqualsDTS: h264.IDRPresent(nalus),
									h264NALUs:    nalus,
									h264PTS:      pts,
								})
							}
						}
					}

				case *message.MsgAudio:
					if tmsg.AACType == flvio.AAC_RAW {
						if audioTrack == nil {
							return fmt.Errorf("received an AAC packet, but track is not set up")
						}

						pkts, err := aacEncoder.Encode([][]byte{tmsg.Payload}, tmsg.DTS)
						if err != nil {
							return fmt.Errorf("error while encoding AAC: %v", err)
						}

						for _, pkt := range pkts {
							res.stream.writeData(&data{
								trackID:      audioTrackID,
								rtp:          pkt,
								ptsEqualsDTS: true,
							})
						}
					}
				}
			}
		}()
	}()

	select {
	case err := <-readDone:
		nconn.Close()
		return err

	case <-innerCtx.Done():
		nconn.Close()
		<-readDone
		return nil
	}
}

// onSourceAPIDescribe implements source.
func (*rtmpSource) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtmpSource"}
}
