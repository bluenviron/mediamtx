package main

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
)

const (
	sourceRtmpRetryInterval = 5 * time.Second
)

type sourceRtmpState int

const (
	sourceRtmpStateStopped sourceRtmpState = iota
	sourceRtmpStateRunning
)

type sourceRtmp struct {
	p            *program
	path         *path
	state        sourceRtmpState
	innerRunning bool

	innerTerminate chan struct{}
	innerDone      chan struct{}
	setState       chan sourceRtmpState
	terminate      chan struct{}
	done           chan struct{}
}

func newSourceRtmp(p *program, path *path) *sourceRtmp {
	s := &sourceRtmp{
		p:         p,
		path:      path,
		setState:  make(chan sourceRtmpState),
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	atomic.AddInt64(p.countSourcesRtmp, +1)

	if path.conf.SourceOnDemand {
		s.state = sourceRtmpStateStopped
	} else {
		s.state = sourceRtmpStateRunning
		atomic.AddInt64(p.countSourcesRtmpRunning, +1)
	}

	return s
}

func (s *sourceRtmp) isPublisher() {}

func (s *sourceRtmp) run(initialState sourceRtmpState) {
	s.applyState(initialState)

outer:
	for {
		select {
		case state := <-s.setState:
			s.applyState(state)

		case <-s.terminate:
			break outer
		}
	}

	if s.innerRunning {
		close(s.innerTerminate)
		<-s.innerDone
	}

	close(s.setState)
	close(s.done)
}

func (s *sourceRtmp) applyState(state sourceRtmpState) {
	if state == sourceRtmpStateRunning {
		if !s.innerRunning {
			s.path.log("rtmp source started")
			s.innerRunning = true
			s.innerTerminate = make(chan struct{})
			s.innerDone = make(chan struct{})
			go s.runInner()
		}
	} else {
		if s.innerRunning {
			close(s.innerTerminate)
			<-s.innerDone
			s.innerRunning = false
			s.path.log("rtmp source stopped")
		}
	}
}

func (s *sourceRtmp) runInner() {
	defer close(s.innerDone)

outer:
	for {
		ok := s.runInnerInner()
		if !ok {
			break outer
		}

		t := time.NewTimer(sourceRtmpRetryInterval)
		defer t.Stop()

		select {
		case <-s.innerTerminate:
			break outer
		case <-t.C:
		}
	}
}

func (s *sourceRtmp) runInnerInner() bool {
	s.path.log("connecting to rtmp source")

	var conn *rtmp.Conn
	var nconn net.Conn
	var err error
	dialDone := make(chan struct{}, 1)
	go func() {
		defer close(dialDone)
		conn, nconn, err = rtmp.NewClient().Dial(s.path.conf.Source, rtmp.PrepareReading)
	}()

	select {
	case <-s.innerTerminate:
		return false
	case <-dialDone:
	}

	if err != nil {
		s.path.log("rtmp source ERR: %s", err)
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
		s.path.log("rtmp source ERR: %s", err)
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
			s.path.log("rtmp source ERR: %s", err)
			return true
		}

		h264Encoder, err = rtph264.NewEncoder(uint8(len(tracks)))
		if err != nil {
			s.path.log("rtmp source ERR: %s", err)
			return true
		}

		tracks = append(tracks, videoTrack)
	}

	if aacConfig != nil {
		audioTrack, err = gortsplib.NewTrackAac(len(tracks), aacConfig)
		if err != nil {
			s.path.log("rtmp source ERR: %s", err)
			return true
		}

		aacEncoder, err = rtpaac.NewEncoder(uint8(len(tracks)), aacConfig)
		if err != nil {
			s.path.log("rtmp source ERR: %s", err)
			return true
		}

		tracks = append(tracks, audioTrack)
	}

	if len(tracks) == 0 {
		s.path.log("rtmp source ERR: no tracks found")
		return true
	}

	s.path.publisherSdp = tracks.Write()
	s.path.publisherTrackCount = len(tracks)

	s.p.sourceRtmpReady <- s
	s.path.log("rtmp source ready")

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
				frames, err := h264Encoder.Write(nalus, pkt.Time)
				if err != nil {
					readDone <- err
					return
				}

				for _, f := range frames {
					s.p.readersMap.forwardFrame(s.path, videoTrack.Id, gortsplib.StreamTypeRtp, f)
				}

			case av.AAC:
				if aacConfig == nil {
					readDone <- fmt.Errorf("rtmp source ERR: received an AAC frame, but track is not setup up")
					return
				}

				frames, err := aacEncoder.Write(pkt.Data, pkt.Time)
				if err != nil {
					readDone <- err
					return
				}

				for _, f := range frames {
					s.p.readersMap.forwardFrame(s.path, audioTrack.Id, gortsplib.StreamTypeRtp, f)
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
			s.path.log("rtmp source ERR: %s", err)
			ret = true
			break outer
		}
	}

	s.p.sourceRtmpNotReady <- s
	s.path.log("rtmp source not ready")

	return ret
}
