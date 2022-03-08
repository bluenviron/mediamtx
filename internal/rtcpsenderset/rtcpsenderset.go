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
	onPacketRTCP func(int, rtcp.Packet)
	senders      []*rtcpsender.RTCPSender

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

// New allocates a RTCPSenderSet.
func New(
	tracks gortsplib.Tracks,
	onPacketRTCP func(int, rtcp.Packet),
) *RTCPSenderSet {
	s := &RTCPSenderSet{
		onPacketRTCP: onPacketRTCP,
		terminate:    make(chan struct{}),
		done:         make(chan struct{}),
	}

	s.senders = make([]*rtcpsender.RTCPSender, len(tracks))
	for i, track := range tracks {
		s.senders[i] = rtcpsender.New(track.ClockRate())
	}

	go s.run()

	return s
}

// Close closes a RTCPSenderSet.
func (s *RTCPSenderSet) Close() {
	close(s.terminate)
	<-s.done
}

func (s *RTCPSenderSet) run() {
	defer close(s.done)

	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			now := time.Now()

			for i, sender := range s.senders {
				r := sender.Report(now)
				if r != nil {
					s.onPacketRTCP(i, r)
				}
			}

		case <-s.terminate:
			return
		}
	}
}

// OnPacketRTP sends a RTP packet to the senders.
func (s *RTCPSenderSet) OnPacketRTP(trackID int, pkt *rtp.Packet) {
	s.senders[trackID].ProcessPacketRTP(time.Now(), pkt)
}
