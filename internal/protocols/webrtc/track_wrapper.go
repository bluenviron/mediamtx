package webrtc

import (
	"github.com/pion/rtp"
)

// TrackWrapper provides ClockRate() and PTSEqualsDTS() to WebRTC tracks.
type TrackWrapper struct {
	ClockRat int
}

// ClockRate returns the clock rate.
func (w TrackWrapper) ClockRate() int {
	return w.ClockRat
}

// PTSEqualsDTS returns whether PTS equals DTS.
func (TrackWrapper) PTSEqualsDTS(*rtp.Packet) bool {
	return true
}
