package sourcertmp

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtcpsender"
	"github.com/aler9/gortsplib/pkg/rtpaac"
	"github.com/aler9/gortsplib/pkg/rtph264"
	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/rtmp"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	retryPause     = 5 * time.Second
	analyzeTimeout = 8 * time.Second
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
	ur     string
	wg     *sync.WaitGroup
	stats  *stats.Stats
	parent Parent

	// in
	terminate chan struct{}
}

// New allocates a Source.
func New(ur string,
	wg *sync.WaitGroup,
	stats *stats.Stats,
	parent Parent) *Source {
	s := &Source{
		ur:        ur,
		wg:        wg,
		stats:     stats,
		parent:    parent,
		terminate: make(chan struct{}),
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

	var conn *rtmp.Conn
	var nconn net.Conn
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)
		conn, nconn, err = rtmp.NewClient().Dial(s.ur, rtmp.PrepareReading)
	}()

	select {
	case <-s.terminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.log(logger.Info, "ERR: %s", err)
		return true
	}

	// gather video and audio features
	var h264Sps []byte
	var h264Pps []byte
	var aacConfig []byte
	confDone := make(chan struct{})
	confClose := uint32(0)
	go func() {
		defer close(confDone)

		for {
			var pkt av.Packet
			pkt, err = conn.ReadPacket()
			if err != nil {
				return
			}

			if atomic.LoadUint32(&confClose) > 0 {
				return
			}

			switch pkt.Type {
			case av.H264DecoderConfig:
				codec, err := h264.FromDecoderConfig(pkt.Data)
				if err != nil {
					panic(err)
				}

				h264Sps, h264Pps = codec.SPS[0], codec.PPS[0]

				if aacConfig != nil {
					return
				}

			case av.AACDecoderConfig:
				aacConfig = pkt.Data

				if h264Sps != nil {
					return
				}
			}
		}
	}()

	timer := time.NewTimer(analyzeTimeout)
	defer timer.Stop()

	select {
	case <-confDone:
	case <-timer.C:
		atomic.StoreUint32(&confClose, 1)
		<-confDone
	}

	if err != nil {
		s.log(logger.Info, "ERR: %s", err)
		return true
	}

	var tracks gortsplib.Tracks

	var videoTrack *gortsplib.Track
	var videoRTCPSender *rtcpsender.RTCPSender
	var h264Encoder *rtph264.Encoder

	var audioTrack *gortsplib.Track
	var audioRTCPSender *rtcpsender.RTCPSender
	var aacEncoder *rtpaac.Encoder

	if h264Sps != nil {
		videoTrack, err = gortsplib.NewTrackH264(96, h264Sps, h264Pps)
		if err != nil {
			s.log(logger.Info, "ERR: %s", err)
			return true
		}

		clockRate, _ := videoTrack.ClockRate()
		videoRTCPSender = rtcpsender.New(clockRate)

		h264Encoder, err = rtph264.NewEncoder(96)
		if err != nil {
			s.log(logger.Info, "ERR: %s", err)
			return true
		}

		tracks = append(tracks, videoTrack)
	}

	if aacConfig != nil {
		audioTrack, err = gortsplib.NewTrackAAC(96, aacConfig)
		if err != nil {
			s.log(logger.Info, "ERR: %s", err)
			return true
		}

		clockRate, _ := audioTrack.ClockRate()
		audioRTCPSender = rtcpsender.New(clockRate)

		aacEncoder, err = rtpaac.NewEncoder(96, clockRate)
		if err != nil {
			s.log(logger.Info, "ERR: %s", err)
			return true
		}

		tracks = append(tracks, audioTrack)
	}

	if len(tracks) == 0 {
		s.log(logger.Info, "ERR: no tracks found")
		return true
	}

	for i, t := range tracks {
		t.ID = i
	}

	s.log(logger.Info, "ready")
	s.parent.OnSourceSetReady(tracks)
	defer s.parent.OnSourceSetNotReady()

	rtcpTerminate := make(chan struct{})
	rtcpDone := make(chan struct{})
	go func() {
		close(rtcpDone)

		t := time.NewTicker(10 * time.Second)
		defer t.Stop()

		for {
			select {
			case <-t.C:
				now := time.Now()

				if videoRTCPSender != nil {
					r := videoRTCPSender.Report(now)
					if r != nil {
						s.parent.OnFrame(videoTrack.ID, gortsplib.StreamTypeRTCP, r)
					}
				}

				if audioRTCPSender != nil {
					r := audioRTCPSender.Report(now)
					if r != nil {
						s.parent.OnFrame(audioTrack.ID, gortsplib.StreamTypeRTCP, r)
					}
				}

			case <-rtcpTerminate:
				return
			}
		}
	}()

	readerDone := make(chan error)
	go func() {
		for {
			pkt, err := conn.ReadPacket()
			if err != nil {
				readerDone <- err
				return
			}

			switch pkt.Type {
			case av.H264:
				if h264Sps == nil {
					readerDone <- fmt.Errorf("rtmp source ERR: received an H264 frame, but track is not setup up")
					return
				}

				// decode from AVCC format
				nalus, typ := h264.SplitNALUs(pkt.Data)
				if typ != h264.NALU_AVCC {
					readerDone <- fmt.Errorf("invalid NALU format (%d)", typ)
					return
				}

				// encode into RTP/H264 format
				frames, err := h264Encoder.Write(pkt.Time+pkt.CTime, nalus)
				if err != nil {
					readerDone <- err
					return
				}

				for _, f := range frames {
					videoRTCPSender.ProcessFrame(time.Now(), gortsplib.StreamTypeRTP, f)
					s.parent.OnFrame(videoTrack.ID, gortsplib.StreamTypeRTP, f)
				}

			case av.AAC:
				if aacConfig == nil {
					readerDone <- fmt.Errorf("rtmp source ERR: received an AAC frame, but track is not setup up")
					return
				}

				frames, err := aacEncoder.Write(pkt.Time+pkt.CTime, pkt.Data)
				if err != nil {
					readerDone <- err
					return
				}

				for _, f := range frames {
					audioRTCPSender.ProcessFrame(time.Now(), gortsplib.StreamTypeRTP, f)
					s.parent.OnFrame(audioTrack.ID, gortsplib.StreamTypeRTP, f)
				}

			default:
				readerDone <- fmt.Errorf("rtmp source ERR: unexpected packet: %v", pkt.Type)
				return
			}
		}
	}()

	for {
		select {
		case <-s.terminate:
			nconn.Close()
			<-readerDone

			close(rtcpTerminate)
			<-rtcpDone
			return false

		case err := <-readerDone:
			nconn.Close()
			s.log(logger.Info, "ERR: %s", err)

			close(rtcpTerminate)
			<-rtcpDone
			return true
		}
	}
}
