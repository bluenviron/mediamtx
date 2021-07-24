package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"

	"github.com/aler9/rtsp-simple-server/internal/h264"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtcpsenderset"
	"github.com/aler9/rtsp-simple-server/internal/rtmp"
)

const (
	rtmpSourceRetryPause = 5 * time.Second
)

type rtmpSourceParent interface {
	Log(logger.Level, string, ...interface{})
	OnsourceExternalSetReady(req sourceExtSetReadyReq)
	OnsourceExternalSetNotReady(req sourceExtSetNotReadyReq)
	OnFrame(int, gortsplib.StreamType, []byte)
}

type rtmpSource struct {
	ur           string
	readTimeout  time.Duration
	writeTimeout time.Duration
	wg           *sync.WaitGroup
	stats        *stats
	parent       rtmpSourceParent

	ctx       context.Context
	ctxCancel func()
}

func newRTMPSource(
	parentCtx context.Context,
	ur string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	wg *sync.WaitGroup,
	stats *stats,
	parent rtmpSourceParent) *rtmpSource {
	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &rtmpSource{
		ur:           ur,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		wg:           wg,
		stats:        stats,
		parent:       parent,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
	}

	atomic.AddInt64(s.stats.CountSourcesRTMP, +1)
	s.log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()

	return s
}

// Close closes a Source.
func (s *rtmpSource) Close() {
	atomic.AddInt64(s.stats.CountSourcesRTMPRunning, -1)
	s.log(logger.Info, "stopped")
	s.ctxCancel()
}

// IsSource implements source.
func (s *rtmpSource) IsSource() {}

// IsSourceExternal implements sourceExternal.
func (s *rtmpSource) IsSourceExternal() {}

func (s *rtmpSource) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[rtmp source] "+format, args...)
}

func (s *rtmpSource) run() {
	defer s.wg.Done()

	for {
		ok := func() bool {
			ok := s.runInner()
			if !ok {
				return false
			}

			select {
			case <-time.After(rtmpSourceRetryPause):
				return true
			case <-s.ctx.Done():
				return false
			}
		}()
		if !ok {
			break
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

			ctx2, cancel2 := context.WithTimeout(innerCtx, s.readTimeout)
			defer cancel2()

			conn, err := rtmp.DialContext(ctx2, s.ur)
			if err != nil {
				return err
			}

			readDone := make(chan error)
			go func() {
				readDone <- func() error {
					conn.NetConn().SetReadDeadline(time.Now().Add(s.readTimeout))
					conn.NetConn().SetWriteDeadline(time.Now().Add(s.writeTimeout))
					err = conn.ClientHandshake()
					if err != nil {
						return err
					}

					conn.NetConn().SetWriteDeadline(time.Time{})

					conn.NetConn().SetReadDeadline(time.Now().Add(s.readTimeout))
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

					s.log(logger.Info, "ready")

					cres := make(chan sourceExtSetReadyRes)
					s.parent.OnsourceExternalSetReady(sourceExtSetReadyReq{
						Tracks: tracks,
						Res:    cres,
					})
					<-cres

					defer func() {
						res := make(chan struct{})
						s.parent.OnsourceExternalSetNotReady(sourceExtSetNotReadyReq{
							Res: res,
						})
						<-res
					}()

					rtcpSenders := rtcpsenderset.New(tracks, s.parent.OnFrame)
					defer rtcpSenders.Close()

					onFrame := func(trackID int, payload []byte) {
						rtcpSenders.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
						s.parent.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
					}

					for {
						conn.NetConn().SetReadDeadline(time.Now().Add(s.readTimeout))
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

							for _, pkt := range pkts {
								onFrame(videoTrackID, pkt)
							}

						case av.AAC:
							if audioTrack == nil {
								return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
							}

							pkts, err := aacEncoder.Encode([][]byte{pkt.Data}, pkt.Time+pkt.CTime)
							if err != nil {
								return fmt.Errorf("ERR while encoding AAC: %v", err)
							}

							for _, pkt := range pkts {
								onFrame(audioTrackID, pkt)
							}

						default:
							return fmt.Errorf("ERR: unexpected packet: %v", pkt.Type)
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
