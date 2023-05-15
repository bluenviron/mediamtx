package core

import (
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const (
	keyFrameInterval = 2 * time.Second
)

type webRTCIncomingTrack struct {
	track     *webrtc.TrackRemote
	receiver  *webrtc.RTPReceiver
	writeRTCP func([]rtcp.Packet) error

	mediaType media.Type
	format    formats.Format
	media     *media.Media
}

func newWebRTCIncomingTrack(
	track *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
	writeRTCP func([]rtcp.Packet) error,
) (*webRTCIncomingTrack, error) {
	t := &webRTCIncomingTrack{
		track:     track,
		receiver:  receiver,
		writeRTCP: writeRTCP,
	}

	switch track.Codec().MimeType {
	case webrtc.MimeTypeAV1:
		t.mediaType = media.TypeVideo
		t.format = &formats.AV1{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case webrtc.MimeTypeVP9:
		t.mediaType = media.TypeVideo
		t.format = &formats.VP9{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case webrtc.MimeTypeVP8:
		t.mediaType = media.TypeVideo
		t.format = &formats.VP8{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case webrtc.MimeTypeH264:
		t.mediaType = media.TypeVideo
		t.format = &formats.H264{
			PayloadTyp:        uint8(track.PayloadType()),
			PacketizationMode: 1,
		}

	case webrtc.MimeTypeOpus:
		t.mediaType = media.TypeAudio
		t.format = &formats.Opus{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case webrtc.MimeTypeG722:
		t.mediaType = media.TypeAudio
		t.format = &formats.G722{}

	case webrtc.MimeTypePCMU:
		t.mediaType = media.TypeAudio
		t.format = &formats.G711{MULaw: true}

	case webrtc.MimeTypePCMA:
		t.mediaType = media.TypeAudio
		t.format = &formats.G711{MULaw: false}

	default:
		return nil, fmt.Errorf("unsupported codec: %v", track.Codec())
	}

	t.media = &media.Media{
		Type:    t.mediaType,
		Formats: []formats.Format{t.format},
	}

	return t, nil
}

func (t *webRTCIncomingTrack) start(stream *stream) {
	go func() {
		for {
			pkt, _, err := t.track.ReadRTP()
			if err != nil {
				return
			}

			stream.writeRTPPacket(t.media, t.format, pkt, time.Now())
		}
	}()

	// read incoming RTCP packets to make interceptors work
	go func() {
		buf := make([]byte, 1500)
		for {
			_, _, err := t.receiver.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	if t.mediaType == media.TypeVideo {
		go func() {
			keyframeTicker := time.NewTicker(keyFrameInterval)
			defer keyframeTicker.Stop()

			for range keyframeTicker.C {
				err := t.writeRTCP([]rtcp.Packet{
					&rtcp.PictureLossIndication{
						MediaSSRC: uint32(t.track.SSRC()),
					},
				})
				if err != nil {
					return
				}
			}
		}()
	}
}
