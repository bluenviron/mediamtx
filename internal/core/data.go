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
type data interface {
	getTrackID() int
	getRTPPacket() *rtp.Packet
	getPTSEqualsDTS() bool
}

type dataGeneric struct {
	trackID      int
	rtpPacket    *rtp.Packet
	ptsEqualsDTS bool
}

func (d *dataGeneric) getTrackID() int {
	return d.trackID
}

func (d *dataGeneric) getRTPPacket() *rtp.Packet {
	return d.rtpPacket
}

func (d *dataGeneric) getPTSEqualsDTS() bool {
	return d.ptsEqualsDTS
}

type dataH264 struct {
	trackID      int
	rtpPacket    *rtp.Packet
	ptsEqualsDTS bool
	pts          time.Duration
	nalus        [][]byte
}

func (d *dataH264) getTrackID() int {
	return d.trackID
}

func (d *dataH264) getRTPPacket() *rtp.Packet {
	return d.rtpPacket
}

func (d *dataH264) getPTSEqualsDTS() bool {
	return d.ptsEqualsDTS
}

type dataMPEG4Audio struct {
	trackID   int
	rtpPacket *rtp.Packet
	pts       time.Duration
	au        []byte
}

func (d *dataMPEG4Audio) getTrackID() int {
	return d.trackID
}

func (d *dataMPEG4Audio) getRTPPacket() *rtp.Packet {
	return d.rtpPacket
}

func (d *dataMPEG4Audio) getPTSEqualsDTS() bool {
	return true
}
