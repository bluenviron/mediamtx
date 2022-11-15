package core

import (
	"time"

	"github.com/pion/rtp"
)

// data is the data unit routed across the server.
type data interface {
	getTrackID() int
	getRTPPackets() []*rtp.Packet
}

type dataGeneric struct {
	trackID    int
	rtpPackets []*rtp.Packet
}

func (d *dataGeneric) getTrackID() int {
	return d.trackID
}

func (d *dataGeneric) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

type dataH264 struct {
	trackID    int
	rtpPackets []*rtp.Packet
	pts        time.Duration
	nalus      [][]byte
}

func (d *dataH264) getTrackID() int {
	return d.trackID
}

func (d *dataH264) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

type dataMPEG4Audio struct {
	trackID    int
	rtpPackets []*rtp.Packet
	pts        time.Duration
	aus        [][]byte
}

func (d *dataMPEG4Audio) getTrackID() int {
	return d.trackID
}

func (d *dataMPEG4Audio) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}
