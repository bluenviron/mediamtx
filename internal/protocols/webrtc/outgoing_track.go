package webrtc

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type addTrackFunc func(webrtc.TrackLocal) (*webrtc.RTPSender, error)

// OutgoingTrack is a WebRTC outgoing track
type OutgoingTrack struct {
	track *webrtc.TrackLocalStaticRTP
}

func newOutgoingTrack(forma format.Format, addTrack addTrackFunc) (*OutgoingTrack, error) {
	t := &OutgoingTrack{}

	switch forma := forma.(type) {
	case *format.AV1:
		var err error
		t.track, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
			"av1",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

	case *format.VP9:
		var err error
		t.track, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP9,
				ClockRate: uint32(forma.ClockRate()),
			},
			"vp9",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

	case *format.VP8:
		var err error
		t.track, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: uint32(forma.ClockRate()),
			},
			"vp8",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

	case *format.H264:
		var err error
		t.track, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH264,
				ClockRate: uint32(forma.ClockRate()),
			},
			"h264",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

	case *format.Opus:
		var err error
		t.track, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: uint32(forma.ClockRate()),
				Channels:  2,
			},
			"opus",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

	case *format.G722:
		var err error
		t.track, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeG722,
				ClockRate: uint32(forma.ClockRate()),
			},
			"g722",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

	case *format.G711:
		var mtyp string
		if forma.MULaw {
			mtyp = webrtc.MimeTypePCMU
		} else {
			mtyp = webrtc.MimeTypePCMA
		}

		var err error
		t.track, err = webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{
				MimeType:  mtyp,
				ClockRate: uint32(forma.ClockRate()),
			},
			"g711",
			webrtcStreamID,
		)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported track type: %T", forma)
	}

	sender, err := addTrack(t.track)
	if err != nil {
		return nil, err
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

	return t, nil
}

// WriteRTP writes a RTP packet.
func (t *OutgoingTrack) WriteRTP(pkt *rtp.Packet) error {
	return t.track.WriteRTP(pkt)
}
