package core

import (
	"time"

	"github.com/pion/rtp"
)

// data is the data unit routed across the server.
// it must contain one or more of the following:
// - a single RTP packet
// - a group of H264 NALUs (grouped by timestamp)
// - a single AAC AU
type data struct {
	trackID int

	rtpPacket *rtp.Packet

	// timing
	ptsEqualsDTS bool
	pts          time.Duration

	h264NALUs [][]byte

	mpeg4AudioAU []byte
}
