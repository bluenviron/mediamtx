package core

import (
	"time"

	"github.com/pion/rtp"
)

type data struct {
	trackID      int
	rtp          *rtp.Packet
	ptsEqualsDTS bool
	h264NALUs    [][]byte
	h264PTS      time.Duration
}
