package sourcertmp

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/rtmp"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/rtmputils"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	retryPause = 5 * time.Second
)

// Parent is implemeneted by path.Path.
type Parent interface {
	Log(logger.Level, string, ...interface{})
	OnSourceSetReady(gortsplib.Tracks)
	OnSourceSetNotReady()
	OnFrame(int, gortsplib.StreamType, []byte)
}

// Source is a RTMP source.
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

// IsSource implements path.source.
func (s *Source) IsSource() {}

// IsSourceExternal implements path.sourceExternal.
func (s *Source) IsSourceExternal() {}

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

			t := time.NewTimer(retryPause)
			defer t.Stop()

			select {
			case <-t.C:
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

	var conn *rtmputils.Conn
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)
		var rconn *rtmp.Conn
		var nconn net.Conn
		rconn, nconn, err = rtmp.NewClient().Dial(s.ur, rtmp.PrepareReading)
		conn = rtmputils.NewConn(rconn, nconn)
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
		videoTrack, audioTrack, err = rtmputils.ReadMetadata(conn)
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
	s.parent.OnSourceSetReady(tracks)
	defer s.parent.OnSourceSetNotReady()

	readerDone := make(chan error)
	go func() {
		readerDone <- func() error {
			rtcpSenders := rtmputils.NewRTCPSenderSet(tracks, s.parent.OnFrame)
			defer rtcpSenders.Close()

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

					// decode from AVCC format
					nalus, typ := h264.SplitNALUs(pkt.Data)
					if typ != h264.NALU_AVCC {
						return fmt.Errorf("invalid NALU format (%d)", typ)
					}

					for _, nalu := range nalus {
						// encode into RTP/H264 format
						frames, err := h264Encoder.Encode(&rtph264.NALUAndTimestamp{
							Timestamp: pkt.Time + pkt.CTime,
							NALU:      nalu,
						})
						if err != nil {
							return err
						}

						for _, frame := range frames {
							rtcpSenders.ProcessFrame(videoTrack.ID, time.Now(),
								gortsplib.StreamTypeRTP, frame)
							s.parent.OnFrame(videoTrack.ID, gortsplib.StreamTypeRTP, frame)
						}
					}

				case av.AAC:
					if audioTrack == nil {
						return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
					}

					frame, err := aacEncoder.Encode(&rtpaac.AUAndTimestamp{
						Timestamp: pkt.Time + pkt.CTime,
						AU:        pkt.Data,
					})
					if err != nil {
						return err
					}

					rtcpSenders.ProcessFrame(audioTrack.ID, time.Now(),
						gortsplib.StreamTypeRTP, frame)
					s.parent.OnFrame(audioTrack.ID, gortsplib.StreamTypeRTP, frame)

				default:
					return fmt.Errorf("ERR: unexpected packet: %v", pkt.Type)
				}
			}
		}()
	}()

	for {
		select {
		case err := <-readerDone:
			conn.NetConn().Close()
			s.log(logger.Info, "ERR: %s", err)
			return true

		case <-s.terminate:
			conn.NetConn().Close()
			<-readerDone
			return false
		}
	}
}
