package webrtc

import (
	"strings"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
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

	track *webrtc.TrackLocalStaticRTP
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

	// read incoming RTCP packets to make interceptors work
	go func() {
		buf := make([]byte, 1500)
		for {
			_, _, err := sender.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	return nil
}

// WriteRTP writes a RTP packet.
func (t *OutgoingTrack) WriteRTP(pkt *rtp.Packet) error {
	return t.track.WriteRTP(pkt)
}
