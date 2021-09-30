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
	"github.com/aler9/rtsp-simple-server/internal/rtcpsenderset"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
)

const (
	rtmpSourceRetryPause = 5 * time.Second
)

type rtmpSourceParent interface {
	Log(logger.Level, string, ...interface{})
	OnSourceStaticSetReady(req pathSourceStaticSetReadyReq) pathSourceStaticSetReadyRes
	OnSourceStaticSetNotReady(req pathSourceStaticSetNotReadyReq)
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
	parent rtmpSourceParent) *rtmpSource {
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
func (s *rtmpSource) Close() {
	s.log(logger.Info, "stopped")
	s.ctxCancel()
}

func (s *rtmpSource) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[rtmp source] "+format, args...)
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
					conn.NetConn().SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
					conn.NetConn().SetWriteDeadline(time.Now().Add(time.Duration(s.writeTimeout)))
					err = conn.ClientHandshake()
					if err != nil {
						return err
					}

					conn.NetConn().SetWriteDeadline(time.Time{})

					conn.NetConn().SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
					videoTrack, audioTrack, err := conn.ReadMetadata()
					if err != nil {
						return err
					}

					var tracks gortsplib.Tracks
					videoTrackID := -1
					audioTrackID := -1

					var h264Encoder *rtph264.Encoder
					if videoTrack != nil {
						h264Encoder = rtph264.NewEncoder(96, nil, nil, nil)
						videoTrackID = len(tracks)
						tracks = append(tracks, videoTrack)
					}

					var aacEncoder *rtpaac.Encoder
					if audioTrack != nil {
						clockRate, _ := audioTrack.ClockRate()
						aacEncoder = rtpaac.NewEncoder(96, clockRate, nil, nil, nil)
						audioTrackID = len(tracks)
						tracks = append(tracks, audioTrack)
					}

					res := s.parent.OnSourceStaticSetReady(pathSourceStaticSetReadyReq{
						Source: s,
						Tracks: tracks,
					})
					if res.Err != nil {
						return res.Err
					}

					s.log(logger.Info, "ready")

					defer func() {
						s.parent.OnSourceStaticSetNotReady(pathSourceStaticSetNotReadyReq{Source: s})
					}()

					rtcpSenders := rtcpsenderset.New(tracks, res.Stream.onFrame)
					defer rtcpSenders.Close()

					onFrame := func(trackID int, payload []byte) {
						rtcpSenders.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
						res.Stream.onFrame(trackID, gortsplib.StreamTypeRTP, payload)
					}

					for {
						conn.NetConn().SetReadDeadline(time.Now().Add(time.Duration(s.readTimeout)))
						pkt, err := conn.ReadPacket()
						if err != nil {
							return err
						}

						switch pkt.Type {
						case av.H264:
							if videoTrack == nil {
								return fmt.Errorf("ERR: received an H264 frame, but track is not set up")
							}

							nalus, err := h264.DecodeAVCC(pkt.Data)
							if err != nil {
								return err
							}

							var outNALUs [][]byte
							for _, nalu := range nalus {
								// remove SPS, PPS and AUD, not needed by RTSP / RTMP
								typ := h264.NALUType(nalu[0] & 0x1F)
								switch typ {
								case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
									continue
								}

								outNALUs = append(outNALUs, nalu)
							}

							pkts, err := h264Encoder.Encode(outNALUs, pkt.Time+pkt.CTime)
							if err != nil {
								return fmt.Errorf("ERR while encoding H264: %v", err)
							}

							bytss := make([][]byte, len(pkts))
							for i, pkt := range pkts {
								byts, err := pkt.Marshal()
								if err != nil {
									return fmt.Errorf("error while encoding H264: %v", err)
								}
								bytss[i] = byts
							}

							for _, byts := range bytss {
								onFrame(videoTrackID, byts)
							}

						case av.AAC:
							if audioTrack == nil {
								return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
							}

							pkts, err := aacEncoder.Encode([][]byte{pkt.Data}, pkt.Time+pkt.CTime)
							if err != nil {
								return fmt.Errorf("ERR while encoding AAC: %v", err)
							}

							bytss := make([][]byte, len(pkts))
							for i, pkt := range pkts {
								byts, err := pkt.Marshal()
								if err != nil {
									return fmt.Errorf("error while encoding AAC: %v", err)
								}
								bytss[i] = byts
							}

							for _, byts := range bytss {
								onFrame(audioTrackID, byts)
							}
						}
					}
				}()
			}()

			select {
			case err := <-readDone:
				conn.NetConn().Close()
				return err

			case <-innerCtx.Done():
				conn.NetConn().Close()
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

// OnSourceAPIDescribe implements source.
func (*rtmpSource) OnSourceAPIDescribe() interface{} {
	return struct {
		Type string `json:"type"`
	}{"rtmpSource"}
}
