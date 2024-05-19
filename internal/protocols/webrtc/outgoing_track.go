package webrtc

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// OutgoingTrack is a WebRTC outgoing track
type OutgoingTrack struct {
	Format format.Format

	track *webrtc.TrackLocalStaticRTP
}

func (t *OutgoingTrack) codecParameters() (webrtc.RTPCodecParameters, error) {
	switch forma := t.Format.(type) {
	case *format.AV1:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
			PayloadType: 96,
		}, nil

	case *format.VP9:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=1",
			},
			PayloadType: 98,
		}, nil

	case *format.VP8:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 99,
		}, nil

	case *format.H264:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			},
			PayloadType: 101,
		}, nil

	case *format.Opus:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
			},
			PayloadType: 111,
		}, nil

	case *format.G722:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeG722,
				ClockRate: 8000,
			},
			PayloadType: 9,
		}, nil

	case *format.G711:
		if forma.MULaw {
			return webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypePCMU,
					ClockRate: 8000,
				},
				PayloadType: 0,
			}, nil
		}

		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypePCMA,
				ClockRate: 8000,
			},
			PayloadType: 8,
		}, nil

	default:
		return webrtc.RTPCodecParameters{}, fmt.Errorf("unsupported track type: %T", forma)
	}
}

func (t *OutgoingTrack) isVideo() bool {
	switch t.Format.(type) {
	case *format.AV1,
		*format.VP9,
		*format.VP8,
		*format.H264:
		return true
	}

	return false
}

func (t *OutgoingTrack) setup(p *PeerConnection) error {
	params, _ := t.codecParameters() //nolint:errcheck

	var trackID string
	if t.isVideo() {
		trackID = "video"
	} else {
		trackID = "audio"
	}

	var err error
	t.track, err = webrtc.NewTrackLocalStaticRTP(
		params.RTPCodecCapability,
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
