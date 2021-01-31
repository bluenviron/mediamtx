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

	var conn rtmputils.ConnPair
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)
		var rconn *rtmp.Conn
		var nconn net.Conn
		rconn, nconn, err = rtmp.NewClient().Dial(s.ur, rtmp.PrepareReading)
		conn = rtmputils.ConnPair{rconn, nconn} //nolint:govet
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
		videoTrack, audioTrack, err = rtmputils.Metadata(
			conn, s.readTimeout) //nolint:govet
	}()

	select {
	case <-metadataDone:
	case <-s.terminate:
		conn.NConn.Close()
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
		var err error
		h264Encoder, err = rtph264.NewEncoder(96)
		if err != nil {
			conn.NConn.Close()
			s.log(logger.Info, "ERR: %s", err)
			return true
		}
		tracks = append(tracks, videoTrack)
	}

	var aacEncoder *rtpaac.Encoder
	if audioTrack != nil {
		clockRate, _ := audioTrack.ClockRate()
		var err error
		aacEncoder, err = rtpaac.NewEncoder(96, clockRate)
		if err != nil {
			conn.NConn.Close()
			s.log(logger.Info, "ERR: %s", err)
			return true
		}
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
				conn.NConn.SetReadDeadline(time.Now().Add(s.readTimeout))
				pkt, err := conn.RConn.ReadPacket()
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

					// encode into RTP/H264 format
					frames, err := h264Encoder.Write(pkt.Time+pkt.CTime, nalus)
					if err != nil {
						return err
					}

					for _, f := range frames {
						rtcpSenders.ProcessFrame(videoTrack.ID, time.Now(), gortsplib.StreamTypeRTP, f)
						s.parent.OnFrame(videoTrack.ID, gortsplib.StreamTypeRTP, f)
					}

				case av.AAC:
					if audioTrack == nil {
						return fmt.Errorf("ERR: received an AAC frame, but track is not set up")
					}

					frames, err := aacEncoder.Write(pkt.Time+pkt.CTime, pkt.Data)
					if err != nil {
						return err
					}

					for _, f := range frames {
						rtcpSenders.ProcessFrame(audioTrack.ID, time.Now(), gortsplib.StreamTypeRTP, f)
						s.parent.OnFrame(audioTrack.ID, gortsplib.StreamTypeRTP, f)
					}

				default:
					return fmt.Errorf("ERR: unexpected packet: %v", pkt.Type)
				}
			}
		}()
	}()

	for {
		select {
		case err := <-readerDone:
			conn.NConn.Close()
			s.log(logger.Info, "ERR: %s", err)
			return true

		case <-s.terminate:
			conn.NConn.Close()
			<-readerDone
			return false
		}
	}
}
