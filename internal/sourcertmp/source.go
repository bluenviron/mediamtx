package sourcertmp

import (
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

// Parent is implemeneted by path.Path.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnExtSourceSetReady(req source.ExtSetReadyReq)
	OnExtSourceSetNotReady(req source.ExtSetNotReadyReq)
}

// Source is a RTMP external source.
type Source struct {
	ur          string
	readTimeout time.Duration
	wg          *sync.WaitGroup
	stats       *stats.Stats
	parent      Parent

	// in
	terminate chan struct{}
}

// New allocates a Source.
func New(ur string,
	readTimeout time.Duration,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Source {
	s := &Source{
		ur:          ur,
		readTimeout: readTimeout,
		wg:          wg,
		stats:       stats,
		parent:      parent,
		terminate:   make(chan struct{}),
	}

	atomic.AddInt64(s.stats.CountSourcesRtmp, +1)
	s.log(logger.Info, "started")

	s.wg.Add(1)
	go s.run()
	return s
}

// Close closes a Source.
func (s *Source) Close() {
	atomic.AddInt64(s.stats.CountSourcesRtmpRunning, -1)
	s.log(logger.Info, "stopped")
	close(s.terminate)
}

// IsSource implements source.Source.
func (s *Source) IsSource() {}

// IsExtSource implements path.extSource.
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
			case <-s.terminate:
				return false
			}
		}()
		if !ok {
			break
		}
	}
}

func (s *Source) runInner() bool {
	s.log(logger.Info, "connecting")

	var conn *rtmp.Conn
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)
		conn, err = rtmp.Dial(s.ur)
	}()

	select {
	case <-dialDone:
	case <-s.terminate:
		return false
	}

	if err != nil {
		s.log(logger.Info, "ERR: %s", err)
		return true
	}

	var videoTrack *gortsplib.Track
	var audioTrack *gortsplib.Track
	metadataDone := make(chan struct{})
	go func() {
		defer close(metadataDone)
		conn.NetConn().SetReadDeadline(time.Now().Add(s.readTimeout))
		videoTrack, audioTrack, err = conn.ReadMetadata()
	}()

	select {
	case <-metadataDone:
	case <-s.terminate:
		conn.NetConn().Close()
		<-metadataDone
		return false
	}

	if err != nil {
		s.log(logger.Info, "ERR: %s", err)
		return true
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

	readerDone := make(chan error)
	go func() {
		readerDone <- func() error {
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

					frames, err := h264Encoder.Encode(outNALUs, pkt.Time+pkt.CTime)
					if err != nil {
						return fmt.Errorf("ERR while encoding H264: %v", err)
					}

					for _, frame := range frames {
						onFrame(videoTrack.ID, frame)
					}

				case av.AAC:
					if audioTrack == nil {
						return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
					}

					frames, err := aacEncoder.Encode([][]byte{pkt.Data}, pkt.Time+pkt.CTime)
					if err != nil {
						return fmt.Errorf("ERR while encoding AAC: %v", err)
					}

					for _, frame := range frames {
						onFrame(audioTrack.ID, frame)
					}

				default:
					return fmt.Errorf("ERR: unexpected packet: %v", pkt.Type)
				}
			}
		}()
	}()

	select {
	case <-s.terminate:
		conn.NetConn().Close()
		<-readerDone
		return false

	case err := <-readerDone:
		s.log(logger.Info, "ERR: %s", err)
		conn.NetConn().Close()
		return true
	}
}
