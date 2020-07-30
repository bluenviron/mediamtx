package main

import (
	"fmt"
	"net"
	"strconv"

	"github.com/aler9/gortsplib"
	"github.com/aler9/sdp/v3"
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

// generate a sdp from scratch
func sdpForServer(tracks []*gortsplib.Track) (*sdp.SessionDescription, []byte) {
	sout := &sdp.SessionDescription{
		SessionName: func() *sdp.SessionName {
			ret := sdp.SessionName("Stream")
			return &ret
		}(),
		Origin: &sdp.Origin{
			Username:       "-",
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "127.0.0.1",
		},
		TimeDescriptions: []sdp.TimeDescription{
			{Timing: sdp.Timing{0, 0}},
		},
	}

	for i, track := range tracks {
		mout := &sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:   track.Media.MediaName.Media,
				Protos:  []string{"RTP", "AVP"}, // override protocol
				Formats: track.Media.MediaName.Formats,
			},
			Bandwidth: track.Media.Bandwidth,
			Attributes: func() []sdp.Attribute {
				var ret []sdp.Attribute

				for _, attr := range track.Media.Attributes {
					if attr.Key == "rtpmap" || attr.Key == "fmtp" {
						ret = append(ret, attr)
					}
				}

				// control attribute is mandatory, and is the path that is appended
				// to the stream path in SETUP
				ret = append(ret, sdp.Attribute{
					Key:   "control",
					Value: "trackID=" + strconv.FormatInt(int64(i), 10),
				})

				return ret
			}(),
		}
		sout.MediaDescriptions = append(sout.MediaDescriptions, mout)
	}

	bytsout, _ := sout.Marshal()
	return sout, bytsout
}
