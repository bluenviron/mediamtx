package core

import (
	"time"

	"github.com/pion/rtp"
)

// data is the data unit routed across the server.
type data interface {
	getRTPPackets() []*rtp.Packet
	getNTP() time.Time
}

type dataGeneric struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
}

func (d *dataGeneric) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataGeneric) getNTP() time.Time {
	return d.ntp
}

type dataH264 struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	nalus      [][]byte
}

func (d *dataH264) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataH264) getNTP() time.Time {
	return d.ntp
}

type dataMPEG4Audio struct {
	rtpPackets []*rtp.Packet
	ntp        time.Time
	pts        time.Duration
	aus        [][]byte
}

func (d *dataMPEG4Audio) getRTPPackets() []*rtp.Packet {
	return d.rtpPackets
}

func (d *dataMPEG4Audio) getNTP() time.Time {
	return d.ntp
}
