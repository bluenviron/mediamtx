package main

import (
	"fmt"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/codec/h264"
	"github.com/notedit/rtmp/format/rtmp"
	"github.com/pion/rtp"
)

const (
	sourceRtmpRetryInterval = 5 * time.Second
	rtpPayloadMaxSize       = 1460 // 1500 - ip header - udp header - rtp header
)

type rtpH264Encoder struct {
	seqnum    uint16
	ssrc      uint32
	initialTs uint32
	started   time.Duration
}

func newRtpH264Encoder() *rtpH264Encoder {
	return &rtpH264Encoder{
		seqnum:    uint16(0),
		ssrc:      rand.Uint32(),
		initialTs: rand.Uint32(),
	}
}

func (e *rtpH264Encoder) Encode(nalus [][]byte, timestamp time.Duration) ([][]byte, error) {
	var frames [][]byte

	if e.started == time.Duration(0) {
		e.started = timestamp
	}

	// rtp/h264 uses a 90khz clock
	rtpTs := e.initialTs + uint32((timestamp-e.started).Seconds()*90000)

	for i, nalu := range nalus {
		naluFrames, err := e.encodeNalu(nalu, rtpTs, (i == len(nalus)-1))
		if err != nil {
			return nil, err
		}
		frames = append(frames, naluFrames...)
	}

	return frames, nil
}

func (e *rtpH264Encoder) encodeNalu(nalu []byte, rtpTs uint32, isFinal bool) ([][]byte, error) {
	// if the NALU fits into the RTP packet, use a single NALU packet
	if len(nalu) < rtpPayloadMaxSize {
		rpkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        0x02,
				PayloadType:    96,
				SequenceNumber: e.seqnum,
				Timestamp:      rtpTs,
				SSRC:           e.ssrc,
			},
			Payload: nalu,
		}
		e.seqnum++

		if isFinal {
			rpkt.Header.Marker = true
		}

		frame, err := rpkt.Marshal()
		if err != nil {
			return nil, err
		}

		return [][]byte{frame}, nil
	}

	// otherwise, use fragmentation units
	// use only FU-A, not FU-B, since we always use non-interleaved mode
	// (set with packetization-mode=1)

	frameCount := (len(nalu) - 1) / (rtpPayloadMaxSize - 2)
	lastFrameSize := (len(nalu) - 1) % (rtpPayloadMaxSize - 2)
	if lastFrameSize > 0 {
		frameCount++
	}
	frames := make([][]byte, frameCount)

	nri := (nalu[0] >> 5) & 0x03
	typ := nalu[0] & 0x1F
	nalu = nalu[1:] // remove header

	for i := 0; i < frameCount; i++ {
		indicator := 0 | (nri << 5) | 28 // FU-A

		start := uint8(0)
		if i == 0 {
			start = 1
		}
		end := uint8(0)
		le := rtpPayloadMaxSize - 2
		if i == (len(frames) - 1) {
			end = 1
			le = lastFrameSize
		}
		header := (start << 7) | (end << 6) | typ

		data := append([]byte{indicator, header}, nalu[:le]...)
		nalu = nalu[le:]

		rpkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        0x02,
				PayloadType:    96,
				SequenceNumber: e.seqnum,
				Timestamp:      rtpTs,
				SSRC:           e.ssrc,
			},
			Payload: data,
		}
		e.seqnum++

		if isFinal && i == (len(frames)-1) {
			rpkt.Header.Marker = true
		}

		frame, err := rpkt.Marshal()
		if err != nil {
			return nil, err
		}

		frames[i] = frame
	}

	return frames, nil
}

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

	// wait for SPS and PPS
	sps, pps, err := func() ([]byte, []byte, error) {
		for {
			pkt, err := conn.ReadPacket()
			if err != nil {
				return nil, nil, err
			}

			if pkt.Type == av.H264DecoderConfig {
				codec, err := h264.FromDecoderConfig(pkt.Data)
				if err != nil {
					panic(err)
				}

				return codec.SPS[0], codec.PPS[0], nil
			}
		}
	}()
	if err != nil {
		s.path.log("rtmp source ERR: %s", err)
		return true
	}

	track := gortsplib.NewTrackH264(0, sps, pps)
	tracks := gortsplib.Tracks{track}
	s.path.publisherSdp = tracks.Write()
	s.path.publisherTrackCount = len(tracks)

	s.p.sourceRtmpReady <- s
	s.path.log("rtmp source ready")

	readDone := make(chan error)
	go func() {
		encoder := newRtpH264Encoder()

		for {
			pkt, err := conn.ReadPacket()
			if err != nil {
				readDone <- err
				return
			}

			if pkt.Type == av.H264 {
				// decode from AVCC format
				nalus, typ := h264.SplitNALUs(pkt.Data)
				if typ != h264.NALU_AVCC {
					readDone <- fmt.Errorf("invalid NALU format (%d)", typ)
					return
				}

				// encode into RTP/H264 format
				frames, err := encoder.Encode(nalus, pkt.Time)
				if err != nil {
					readDone <- err
					return
				}

				for _, f := range frames {
					s.p.readersMap.forwardFrame(s.path, 0, gortsplib.StreamTypeRtp, f)
				}
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
