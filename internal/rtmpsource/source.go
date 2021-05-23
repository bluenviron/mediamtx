package rtmpsource

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
	"github.com/aler9/rtsp-simple-server/internal/source"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	retryPause = 5 * time.Second
)

// Parent is implemented by path.Path.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnExtSourceSetReady(req source.ExtSetReadyReq)
	OnExtSourceSetNotReady(req source.ExtSetNotReadyReq)
}

// Source is a RTMP external source.
type Source struct {
	ur           string
	readTimeout  time.Duration
	writeTimeout time.Duration
	wg           *sync.WaitGroup
	stats        *stats.Stats
	parent       Parent

	ctx       context.Context
	ctxCancel func()
}

// New allocates a Source.
func New(
	ctxParent context.Context,
	ur string,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Source {
	ctx, ctxCancel := context.WithCancel(ctxParent)

	s := &Source{
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
func (s *Source) Close() {
	atomic.AddInt64(s.stats.CountSourcesRTMPRunning, -1)
	s.log(logger.Info, "stopped")
	s.ctxCancel()
}

// IsSource implements source.Source.
func (s *Source) IsSource() {}

// IsExtSource implements source.ExtSource.
func (s *Source) IsExtSource() {}

func (s *Source) log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[rtmp source] "+format, args...)
}

func (s *Source) run() {
	defer s.wg.Done()

	for {
		ok := func() bool {
			ok := s.runInner()
			if !ok {
				return false
			}

			select {
			case <-time.After(retryPause):
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

func (s *Source) runInner() bool {
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

					var h264Encoder *rtph264.Encoder
					if videoTrack != nil {
						h264Encoder = rtph264.NewEncoder(96, nil, nil, nil)
						tracks = append(tracks, videoTrack)
					}

					var aacEncoder *rtpaac.Encoder
					if audioTrack != nil {
						clockRate, _ := audioTrack.ClockRate()
						aacEncoder = rtpaac.NewEncoder(96, clockRate, nil, nil, nil)
						tracks = append(tracks, audioTrack)
					}

					for i, t := range tracks {
						t.ID = i
					}

					s.log(logger.Info, "ready")

					cres := make(chan source.ExtSetReadyRes)
					s.parent.OnExtSourceSetReady(source.ExtSetReadyReq{
						Tracks: tracks,
						Res:    cres,
					})
					res := <-cres

					defer func() {
						res := make(chan struct{})
						s.parent.OnExtSourceSetNotReady(source.ExtSetNotReadyReq{
							Res: res,
						})
						<-res
					}()

					rtcpSenders := rtcpsenderset.New(tracks, res.SP.OnFrame)
					defer rtcpSenders.Close()

					onFrame := func(trackID int, payload []byte) {
						rtcpSenders.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
						res.SP.OnFrame(trackID, gortsplib.StreamTypeRTP, payload)
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
								onFrame(videoTrack.ID, pkt)
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
								onFrame(audioTrack.ID, pkt)
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
