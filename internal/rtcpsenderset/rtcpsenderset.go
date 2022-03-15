package rtcpsenderset

import (
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtcpsender"
	"github.com/pion/rtcp"
	"github.com/pion/rtp/v2"
)

// RTCPSenderSet is a set of RTCP senders.
type RTCPSenderSet struct {
	writePacketRTCP func(int, rtcp.Packet)
	senders         []*rtcpsender.RTCPSender

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

// New allocates a RTCPSenderSet.
func New(
	tracks gortsplib.Tracks,
	writePacketRTCP func(int, rtcp.Packet),
) *RTCPSenderSet {
	s := &RTCPSenderSet{
		writePacketRTCP: writePacketRTCP,
		terminate:       make(chan struct{}),
		done:            make(chan struct{}),
	}

	s.senders = make([]*rtcpsender.RTCPSender, len(tracks))
	for i, track := range tracks {
		ci := i

		s.senders[i] = rtcpsender.New(10*time.Second,
			track.ClockRate(), func(pkt rtcp.Packet) {
				writePacketRTCP(ci, pkt)
			})
	}

	return s
}

// Close closes a RTCPSenderSet.
func (s *RTCPSenderSet) Close() {
	for _, sender := range s.senders {
		sender.Close()
	}
}

// OnPacketRTP sends a RTP packet to the senders.
func (s *RTCPSenderSet) OnPacketRTP(trackID int, pkt *rtp.Packet) {
	s.senders[trackID].ProcessPacketRTP(time.Now(), pkt)
}
