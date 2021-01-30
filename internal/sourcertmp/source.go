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
	"github.com/notedit/rtmp/format/flv/flvio"
	"github.com/notedit/rtmp/format/rtmp"

	"github.com/aler9/rtsp-simple-server/internal/logger"
	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	retryPause = 5 * time.Second

	codecH264 = 7
	codecAAC  = 10
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

func readMetadata(conn *rtmp.Conn) (flvio.AMFMap, error) {
	pkt, err := conn.ReadPacket()
	if err != nil {
		return nil, err
	}

	if pkt.Type != av.Metadata {
		return nil, fmt.Errorf("first packet must be metadata")
	}

	arr, err := flvio.ParseAMFVals(pkt.Data, false)
	if err != nil {
		return nil, err
	}

	if len(arr) != 1 {
		return nil, fmt.Errorf("invalid metadata")
	}

	ma, ok := arr[0].(flvio.AMFMap)
	if !ok {
		return nil, fmt.Errorf("invalid metadata")
	}

	return ma, nil
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

	var tracks gortsplib.Tracks

	var videoTrack *gortsplib.Track
	var videoRTCPSender *rtcpsender.RTCPSender
	var h264Encoder *rtph264.Encoder

	var audioTrack *gortsplib.Track
	var audioRTCPSender *rtcpsender.RTCPSender
	var aacEncoder *rtpaac.Encoder

	confDone := make(chan error)
	go func() {
		confDone <- func() error {
			md, err := readMetadata(conn)
			if err != nil {
				return err
			}

			hasVideo := false
			if v, ok := md.GetFloat64("videocodecid"); ok {
				switch v {
				case codecH264:
					hasVideo = true
				case 0:
				default:
					return fmt.Errorf("unsupported video codec %v", v)
				}

			}

			hasAudio := false
			if v, ok := md.GetFloat64("audiocodecid"); ok {
				switch v {
				case codecAAC:
					hasAudio = true
				case 0:
				default:
					return fmt.Errorf("unsupported audio codec %v", v)
				}
			}

			if !hasVideo && !hasAudio {
				return fmt.Errorf("stream has no tracks")
			}

			for {
				var pkt av.Packet
				pkt, err = conn.ReadPacket()
				if err != nil {
					return err
				}

				switch pkt.Type {
				case av.H264DecoderConfig:
					if !hasVideo {
						return fmt.Errorf("unexpected video packet")
					}
					if videoTrack != nil {
						return fmt.Errorf("video track setupped twice")
					}

					codec, err := h264.FromDecoderConfig(pkt.Data)
					if err != nil {
						return err
					}

					videoTrack, err = gortsplib.NewTrackH264(96, codec.SPS[0], codec.PPS[0])
					if err != nil {
						return err
					}

					clockRate, _ := videoTrack.ClockRate()
					videoRTCPSender = rtcpsender.New(clockRate)

					h264Encoder, err = rtph264.NewEncoder(96)
					if err != nil {
						return err
					}

					tracks = append(tracks, videoTrack)

				case av.AACDecoderConfig:
					if !hasAudio {
						return fmt.Errorf("unexpected audio packet")
					}
					if audioTrack != nil {
						return fmt.Errorf("audio track setupped twice")
					}

					audioTrack, err = gortsplib.NewTrackAAC(96, pkt.Data)
					if err != nil {
						return err
					}

					clockRate, _ := audioTrack.ClockRate()
					audioRTCPSender = rtcpsender.New(clockRate)

					aacEncoder, err = rtpaac.NewEncoder(96, clockRate)
					if err != nil {
						return err
					}

					tracks = append(tracks, audioTrack)
				}

				if (!hasVideo || videoTrack != nil) &&
					(!hasAudio || audioTrack != nil) {
					return nil
				}
			}
		}()
	}()

	select {
	case err := <-confDone:
		if err != nil {
			s.log(logger.Info, "ERR: %s", err)
			return true
		}

	case <-s.terminate:
		nconn.Close()
		<-confDone
		return false
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
		readerDone <- func() error {
			for {
				pkt, err := conn.ReadPacket()
				if err != nil {
					return err
				}

				switch pkt.Type {
				case av.H264:
					if videoTrack == nil {
						return fmt.Errorf("rtmp source ERR: received an H264 frame, but track is not setup up")
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
						videoRTCPSender.ProcessFrame(time.Now(), gortsplib.StreamTypeRTP, f)
						s.parent.OnFrame(videoTrack.ID, gortsplib.StreamTypeRTP, f)
					}

				case av.AAC:
					if audioTrack == nil {
						return fmt.Errorf("rtmp source ERR: received an AAC frame, but track is not setup up")
					}

					frames, err := aacEncoder.Write(pkt.Time+pkt.CTime, pkt.Data)
					if err != nil {
						return err
					}

					for _, f := range frames {
						audioRTCPSender.ProcessFrame(time.Now(), gortsplib.StreamTypeRTP, f)
						s.parent.OnFrame(audioTrack.ID, gortsplib.StreamTypeRTP, f)
					}

				default:
					return fmt.Errorf("rtmp source ERR: unexpected packet: %v", pkt.Type)
				}
			}
		}()
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
