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

	"github.com/aler9/rtsp-simple-server/internal/stats"
)

const (
	retryPause     = 5 * time.Second
	analyzeTimeout = 8 * time.Second
)

// Parent is implemeneted by path.Path.
type Parent interface {
	Log(string, ...interface{})
	OnSourceSetReady(gortsplib.Tracks)
	OnSourceSetNotReady()
	OnFrame(int, gortsplib.StreamType, []byte)
}

// Source is a RTMP source.
type Source struct {
	ur     string
	state  bool
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
	s.parent.Log("rtmp source started")

	s.wg.Add(1)
	go s.run()
	return s
}

// Close closes a Source.
func (s *Source) Close() {
	atomic.AddInt64(s.stats.CountSourcesRtmpRunning, -1)
	s.parent.Log("rtmp source stopped")
	close(s.terminate)
}

// IsSource implements path.source.
func (s *Source) IsSource() {}

// IsSourceExternal implements path.sourceExternal.
func (s *Source) IsSourceExternal() {}

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
	s.parent.Log("connecting to rtmp source")

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
		s.parent.Log("rtmp source ERR: %s", err)
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
		s.parent.Log("rtmp source ERR: %s", err)
		return true
	}

	var tracks gortsplib.Tracks
	var videoTrack *gortsplib.Track
	var audioTrack *gortsplib.Track
	var h264Encoder *rtph264.Encoder
	var aacEncoder *rtpaac.Encoder

	if h264Sps != nil {
		videoTrack, err = gortsplib.NewTrackH264(len(tracks), h264Sps, h264Pps)
		if err != nil {
			s.parent.Log("rtmp source ERR: %s", err)
			return true
		}

		h264Encoder, err = rtph264.NewEncoder(uint8(len(tracks)))
		if err != nil {
			s.parent.Log("rtmp source ERR: %s", err)
			return true
		}

		tracks = append(tracks, videoTrack)
	}

	if aacConfig != nil {
		audioTrack, err = gortsplib.NewTrackAac(len(tracks), aacConfig)
		if err != nil {
			s.parent.Log("rtmp source ERR: %s", err)
			return true
		}

		aacEncoder, err = rtpaac.NewEncoder(uint8(len(tracks)), aacConfig)
		if err != nil {
			s.parent.Log("rtmp source ERR: %s", err)
			return true
		}

		tracks = append(tracks, audioTrack)
	}

	if len(tracks) == 0 {
		s.parent.Log("rtmp source ERR: no tracks found")
		return true
	}

	s.parent.Log("rtmp source ready")
	s.parent.OnSourceSetReady(tracks)
	defer s.parent.OnSourceSetNotReady()

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
				frames, err := h264Encoder.Write(nalus, pkt.Time+pkt.CTime)
				if err != nil {
					readerDone <- err
					return
				}

				for _, f := range frames {
					s.parent.OnFrame(videoTrack.Id, gortsplib.StreamTypeRtp, f)
				}

			case av.AAC:
				if aacConfig == nil {
					readerDone <- fmt.Errorf("rtmp source ERR: received an AAC frame, but track is not setup up")
					return
				}

				frames, err := aacEncoder.Write(pkt.Data, pkt.Time+pkt.CTime)
				if err != nil {
					readerDone <- err
					return
				}

				for _, f := range frames {
					s.parent.OnFrame(audioTrack.Id, gortsplib.StreamTypeRtp, f)
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
			return false

		case err := <-readerDone:
			nconn.Close()
			s.parent.Log("rtmp source ERR: %s", err)
			return true
		}
	}
}
