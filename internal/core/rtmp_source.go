package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"

	"github.com/aler9/rtsp-simple-server/internal/conf"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
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

	s.log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()

	return s
}

// Close closes a Source.
func (s *rtmpSource) close() {
	s.log(logger.Info, "stopped")
	s.ctxCancel()
}

func (s *rtmpSource) log(level logger.Level, format string, args ...interface{}) {
	s.parent.log(level, "[rtmp source] "+format, args...)
}

func (s *rtmpSource) run() {
	defer s.wg.Done()

outer:
	for {
		ok := s.runInner()
		if !ok {
			break outer
		}

		select {
		case <-time.After(rtmpSourceRetryPause):
		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()
}

func (s *rtmpSource) runInner() bool {
	innerCtx, innerCtxCancel := context.WithCancel(s.ctx)

	runErr := make(chan error)
	go func() {
		runErr <- func() error {
			s.log(logger.Debug, "connecting")

			ctx2, cancel2 := context.WithTimeout(innerCtx, time.Duration(s.readTimeout))
			defer cancel2()

			conn, err := rtmp.DialContext(ctx2, s.ur)
			if err != nil {
				return err
			}

			readDone := make(chan error)
			go func() {
				readDone <- func() error {
					conn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
					conn.SetWriteDeadline(time.Now().Add(time.Duration(s.writeTimeout)))
					err = conn.ClientHandshake()
					if err != nil {
						return err
					}

					conn.SetWriteDeadline(time.Time{})
					conn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
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

					s.log(logger.Info, "ready")

					defer func() {
						s.parent.onSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{source: s})
					}()

					for {
						conn.SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
						pkt, err := conn.ReadPacket()
						if err != nil {
							return err
						}

						switch pkt.Type {
						case av.H264:
							if videoTrack == nil {
								return fmt.Errorf("received an H264 packet, but track is not set up")
							}

							nalus, err := h264.AVCCUnmarshal(pkt.Data)
							if err != nil {
								return err
							}

							pts := pkt.Time + pkt.CTime

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

						case av.AAC:
							if audioTrack == nil {
								return fmt.Errorf("received an AAC packet, but track is not set up")
							}

							pkts, err := aacEncoder.Encode([][]byte{pkt.Data}, pkt.Time+pkt.CTime)
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
				}()
			}()

			select {
			case err := <-readDone:
				conn.Close()
				return err

			case <-innerCtx.Done():
				conn.Close()
				<-readDone
				return nil
			}
		}()
	}()

	select {
	case err := <-runErr:
		innerCtxCancel()
		s.log(logger.Info, "ERR: %s", err)
		return true

	case <-s.ctx.Done():
		innerCtxCancel()
		<-runErr
		return false
	}
}

// onSourceAPIDescribe implements source.
func (*rtmpSource) onSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtmpSource"}
}
