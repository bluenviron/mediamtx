package webrtc

import (
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/rtcpsender"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

var multichannelOpusSDP = map[int]string{
	3: "channel_mapping=0,2,1;num_streams=2;coupled_streams=1",
	4: "channel_mapping=0,1,2,3;num_streams=2;coupled_streams=2",
	5: "channel_mapping=0,4,1,2,3;num_streams=3;coupled_streams=2",
	6: "channel_mapping=0,4,1,2,3,5;num_streams=4;coupled_streams=2",
	7: "channel_mapping=0,4,1,2,3,5,6;num_streams=4;coupled_streams=4",
	8: "channel_mapping=0,6,1,4,5,2,3,7;num_streams=5;coupled_streams=4",
}

// OutgoingTrack is a WebRTC outgoing track
type OutgoingTrack struct {
	Caps webrtc.RTPCodecCapability

	track      *webrtc.TrackLocalStaticRTP
	ssrc       uint32
	rtcpSender *rtcpsender.RTCPSender
}

func (t *OutgoingTrack) isVideo() bool {
	return strings.Split(t.Caps.MimeType, "/")[0] == "video"
}

func (t *OutgoingTrack) setup(p *PeerConnection) error {
	var trackID string
	if t.isVideo() {
		trackID = "video"
	} else {
		trackID = "audio"
	}

	var err error
	t.track, err = webrtc.NewTrackLocalStaticRTP(
		t.Caps,
		trackID,
		webrtcStreamID,
	)
	if err != nil {
		return err
	}

	sender, err := p.wr.AddTrack(t.track)
	if err != nil {
		return err
	}

	t.ssrc = uint32(sender.GetParameters().Encodings[0].SSRC)

	t.rtcpSender = &rtcpsender.RTCPSender{
		ClockRate: int(t.track.Codec().ClockRate),
		Period:    1 * time.Second,
		TimeNow:   time.Now,
		WritePacketRTCP: func(pkt rtcp.Packet) {
			p.wr.WriteRTCP([]rtcp.Packet{pkt}) //nolint:errcheck
		},
	}
	t.rtcpSender.Initialize()

	p.wr.GetSenders()

	// incoming RTCP packets must always be read to make interceptors work
	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err := sender.Read(buf)
			if err != nil {
				return
			}

			_, err = rtcp.Unmarshal(buf[:n])
			if err != nil {
				panic(err)
			}
		}
	}()

	return nil
}

func (t *OutgoingTrack) close() {
	if t.rtcpSender != nil {
		t.rtcpSender.Close()
	}
}

// WriteRTP writes a RTP packet.
func (t *OutgoingTrack) WriteRTP(pkt *rtp.Packet) error {
	return t.WriteRTPWithNTP(pkt, time.Now())
}

// WriteRTPWithNTP writes a RTP packet.
func (t *OutgoingTrack) WriteRTPWithNTP(pkt *rtp.Packet, ntp time.Time) error {
	// use right SSRC in packet to make rtcpSender work
	pkt.SSRC = t.ssrc

	t.rtcpSender.ProcessPacket(pkt, ntp, true)

	return t.track.WriteRTP(pkt)
}
