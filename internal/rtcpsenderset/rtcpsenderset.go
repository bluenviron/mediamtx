package rtcpsenderset

import (
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/rtcpsender"
)

// RTCPSenderSet is a set of RTCP senders.
type RTCPSenderSet struct {
	onFrame func(int, gortsplib.StreamType, []byte)
	senders []*rtcpsender.RTCPSender

	// in
	terminate chan struct{}

	// out
	done chan struct{}
}

// New allocates a RTCPSenderSet.
func New(
	tracks gortsplib.Tracks,
	onFrame func(int, gortsplib.StreamType, []byte),
) *RTCPSenderSet {
	s := &RTCPSenderSet{
		onFrame:   onFrame,
		terminate: make(chan struct{}),
		done:      make(chan struct{}),
	}

	s.senders = make([]*rtcpsender.RTCPSender, len(tracks))
	for i, t := range tracks {
		clockRate, _ := t.ClockRate()
		s.senders[i] = rtcpsender.New(clockRate)
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
					s.onFrame(i, gortsplib.StreamTypeRTCP, r)
				}
			}

		case <-s.terminate:
			return
		}
	}
}

// OnFrame sends a frame to the senders.
func (s *RTCPSenderSet) OnFrame(trackID int, streamType gortsplib.StreamType, f []byte) {
	s.senders[trackID].ProcessFrame(time.Now(), streamType, f)
}
