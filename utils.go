package main

import (
	"fmt"
	"net"
)

func parseIpCidrList(in []string) ([]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}

	var ret []interface{}
	for _, t := range in {
		_, ipnet, err := net.ParseCIDR(t)
		if err == nil {
			ret = append(ret, ipnet)
			continue
		}

		ip := net.ParseIP(t)
		if ip != nil {
			ret = append(ret, ip)
			continue
		}

		return nil, fmt.Errorf("unable to parse ip/network '%s'", t)
	}
	return ret, nil
}

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}

type trackFlowType int

const (
	_TRACK_FLOW_TYPE_RTP trackFlowType = iota
	_TRACK_FLOW_TYPE_RTCP
)

func interleavedChannelToTrackFlowType(channel uint8) (int, trackFlowType) {
	if (channel % 2) == 0 {
		return int(channel / 2), _TRACK_FLOW_TYPE_RTP
	}
	return int((channel - 1) / 2), _TRACK_FLOW_TYPE_RTCP
}

func trackFlowTypeToInterleavedChannel(id int, trackFlowType trackFlowType) uint8 {
	if trackFlowType == _TRACK_FLOW_TYPE_RTP {
		return uint8(id * 2)
	}
	return uint8((id * 2) + 1)
}

type doubleBuffer struct {
	buf1   []byte
	buf2   []byte
	curBuf bool
}

func newDoubleBuffer(size int) *doubleBuffer {
	return &doubleBuffer{
		buf1: make([]byte, size),
		buf2: make([]byte, size),
	}
}

func (db *doubleBuffer) swap() []byte {
	var ret []byte
	if !db.curBuf {
		ret = db.buf1
	} else {
		ret = db.buf2
	}
	db.curBuf = !db.curBuf
	return ret
}
