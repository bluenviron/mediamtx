package sourcertmp

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/rtpaac"
	"github.com/aler9/gortsplib/rtph264"
	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/rtmp"

	"github.com/aler9/rtsp-simple-server/stats"
)

const (
	retryInterval = 5 * time.Second
)

type Parent interface {
	Log(string, ...interface{})
	OnSourceReady(gortsplib.Tracks)
	OnSourceNotReady()
	OnFrame(int, gortsplib.StreamType, []byte)
}

type Source struct {
	ur     string
	state  bool
	stats  *stats.Stats
	parent Parent

	innerState bool

	// in
	innerTerminate chan struct{}
	innerDone      chan struct{}
	stateChange    chan bool
	terminate      chan struct{}

	// out
	done chan struct{}
}

func New(ur string,
	state bool,
	stats *stats.Stats,
	parent Parent) *Source {
	s := &Source{
		ur:          ur,
		state:       state,
		stats:       stats,
		parent:      parent,
		stateChange: make(chan bool),
		terminate:   make(chan struct{}),
		done:        make(chan struct{}),
	}

	atomic.AddInt64(s.stats.CountSourcesRtmp, +1)

	go s.run()
	s.SetRunning(s.state)
	return s
}

func (s *Source) Close() {
	close(s.terminate)
	<-s.done
}

func (s *Source) IsSource() {}

func (s *Source) IsRunning() bool {
	return s.state
}

func (s *Source) SetRunning(state bool) {
	s.state = state
	s.stateChange <- s.state
}

func (s *Source) run() {
	defer close(s.done)

outer:
	for {
		select {
		case state := <-s.stateChange:
			if state {
				if !s.innerState {
					atomic.AddInt64(s.stats.CountSourcesRtmpRunning, +1)
					s.innerState = true
					s.innerTerminate = make(chan struct{})
					s.innerDone = make(chan struct{})
					go s.runInner()
				}
			} else {
				if s.innerState {
					atomic.AddInt64(s.stats.CountSourcesRtmpRunning, -1)
					close(s.innerTerminate)
					<-s.innerDone
					s.innerState = false
				}
			}

		case <-s.terminate:
			break outer
		}
	}

	if s.innerState {
		atomic.AddInt64(s.stats.CountSourcesRtmpRunning, -1)
		close(s.innerTerminate)
		<-s.innerDone
	}

	close(s.stateChange)
}

func (s *Source) runInner() {
	defer close(s.innerDone)

outer:
	for {
		ok := s.runInnerInner()
		if !ok {
			break outer
		}

		t := time.NewTimer(retryInterval)
		defer t.Stop()

		select {
		case <-s.innerTerminate:
			break outer
		case <-t.C:
		}
	}
}

func (s *Source) runInnerInner() bool {
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
	case <-s.innerTerminate:
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

				h264Sps = codec.SPS[0]
				h264Pps = codec.PPS[0]

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

	timer := time.NewTimer(5 * time.Second)

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

	s.parent.OnSourceReady(tracks)
	s.parent.Log("rtmp source ready")

	readDone := make(chan error)
	go func() {
		for {
			pkt, err := conn.ReadPacket()
			if err != nil {
				readDone <- err
				return
			}

			switch pkt.Type {
			case av.H264:
				if h264Sps == nil {
					readDone <- fmt.Errorf("rtmp source ERR: received an H264 frame, but track is not setup up")
					return
				}

				// decode from AVCC format
				nalus, typ := h264.SplitNALUs(pkt.Data)
				if typ != h264.NALU_AVCC {
					readDone <- fmt.Errorf("invalid NALU format (%d)", typ)
					return
				}

				// encode into RTP/H264 format
				frames, err := h264Encoder.Write(nalus, pkt.Time+pkt.CTime)
				if err != nil {
					readDone <- err
					return
				}

				for _, f := range frames {
					s.parent.OnFrame(videoTrack.Id, gortsplib.StreamTypeRtp, f)
				}

			case av.AAC:
				if aacConfig == nil {
					readDone <- fmt.Errorf("rtmp source ERR: received an AAC frame, but track is not setup up")
					return
				}

				frames, err := aacEncoder.Write(pkt.Data, pkt.Time+pkt.CTime)
				if err != nil {
					readDone <- err
					return
				}

				for _, f := range frames {
					s.parent.OnFrame(audioTrack.Id, gortsplib.StreamTypeRtp, f)
				}

			default:
				readDone <- fmt.Errorf("rtmp source ERR: unexpected packet: %v", pkt.Type)
				return
			}
		}
	}()

	var ret bool

outer:
	for {
		select {
		case <-s.innerTerminate:
			nconn.Close()
			<-readDone
			ret = false
			break outer

		case err := <-readDone:
			nconn.Close()
			s.parent.Log("rtmp source ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.parent.OnSourceNotReady()

	return ret
}
