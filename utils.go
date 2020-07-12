package main

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/sdp"
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

type rtcpReceiverEvent interface {
	isRtpReceiverEvent()
}

type rtcpReceiverEventFrameRtp struct {
	sequenceNumber uint16
}

func (rtcpReceiverEventFrameRtp) isRtpReceiverEvent() {}

type rtcpReceiverEventFrameRtcp struct {
	ssrc          uint32
	ntpTimeMiddle uint32
}

func (rtcpReceiverEventFrameRtcp) isRtpReceiverEvent() {}

type rtcpReceiverEventLastFrameTime struct {
	res chan time.Time
}

func (rtcpReceiverEventLastFrameTime) isRtpReceiverEvent() {}

type rtcpReceiverEventReport struct {
	res chan []byte
}

func (rtcpReceiverEventReport) isRtpReceiverEvent() {}

type rtcpReceiverEventTerminate struct{}

func (rtcpReceiverEventTerminate) isRtpReceiverEvent() {}

type rtcpReceiver struct {
	events chan rtcpReceiverEvent
	done   chan struct{}
}

func newRtcpReceiver() *rtcpReceiver {
	rr := &rtcpReceiver{
		events: make(chan rtcpReceiverEvent),
		done:   make(chan struct{}),
	}
	go rr.run()
	return rr
}

func (rr *rtcpReceiver) run() {
	lastFrameTime := time.Now()
	publisherSSRC := uint32(0)
	receiverSSRC := rand.Uint32()
	sequenceNumberCycles := uint16(0)
	lastSequenceNumber := uint16(0)
	lastSenderReport := uint32(0)

outer:
	for rawEvt := range rr.events {
		switch evt := rawEvt.(type) {
		case rtcpReceiverEventFrameRtp:
			if evt.sequenceNumber < lastSequenceNumber {
				sequenceNumberCycles += 1
			}
			lastSequenceNumber = evt.sequenceNumber
			lastFrameTime = time.Now()

		case rtcpReceiverEventFrameRtcp:
			publisherSSRC = evt.ssrc
			lastSenderReport = evt.ntpTimeMiddle

		case rtcpReceiverEventLastFrameTime:
			evt.res <- lastFrameTime

		case rtcpReceiverEventReport:
			rr := &rtcp.ReceiverReport{
				SSRC: receiverSSRC,
				Reports: []rtcp.ReceptionReport{
					{
						SSRC:               publisherSSRC,
						LastSequenceNumber: uint32(sequenceNumberCycles)<<8 | uint32(lastSequenceNumber),
						LastSenderReport:   lastSenderReport,
					},
				},
			}
			frame, _ := rr.Marshal()
			evt.res <- frame

		case rtcpReceiverEventTerminate:
			break outer
		}
	}

	close(rr.events)

	close(rr.done)
}

func (rr *rtcpReceiver) close() {
	rr.events <- rtcpReceiverEventTerminate{}
	<-rr.done
}

func (rr *rtcpReceiver) onFrame(trackFlowType trackFlowType, buf []byte) {
	if trackFlowType == _TRACK_FLOW_TYPE_RTP {
		// extract sequence number of first frame
		if len(buf) >= 3 {
			sequenceNumber := uint16(uint16(buf[2])<<8 | uint16(buf[1]))
			rr.events <- rtcpReceiverEventFrameRtp{sequenceNumber}
		}

	} else {
		frames, err := rtcp.Unmarshal(buf)
		if err == nil {
			for _, frame := range frames {
				if senderReport, ok := (frame).(*rtcp.SenderReport); ok {
					rr.events <- rtcpReceiverEventFrameRtcp{
						senderReport.SSRC,
						uint32(senderReport.NTPTime >> 16),
					}
				}
			}
		}
	}
}

func (rr *rtcpReceiver) lastFrameTime() time.Time {
	res := make(chan time.Time)
	rr.events <- rtcpReceiverEventLastFrameTime{res}
	return <-res
}

func (rr *rtcpReceiver) report() []byte {
	res := make(chan []byte)
	rr.events <- rtcpReceiverEventReport{res}
	return <-res
}

func sdpFindAttribute(attributes []sdp.Attribute, key string) string {
	for _, attr := range attributes {
		if attr.Key == key {
			return attr.Value
		}
	}
	return ""
}

func sdpForServer(sin *sdp.SessionDescription, bytsin []byte) (*sdp.SessionDescription, []byte) {
	sout := &sdp.SessionDescription{
		SessionName: "Stream",
		Origin: sdp.Origin{
			Username:       "-",
			NetworkType:    "IN",
			AddressType:    "IP4",
			UnicastAddress: "127.0.0.1",
		},
		TimeDescriptions: []sdp.TimeDescription{
			{Timing: sdp.Timing{0, 0}},
		},
	}

	for i, min := range sin.MediaDescriptions {
		mout := &sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:   min.MediaName.Media,
				Protos:  []string{"RTP", "AVP"}, // override protocol
				Formats: min.MediaName.Formats,
			},
			Bandwidth: min.Bandwidth,
			Attributes: func() []sdp.Attribute {
				var ret []sdp.Attribute

				for _, attr := range min.Attributes {
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

	bytsout := []byte(sout.Marshal())
	return sout, bytsout
}
