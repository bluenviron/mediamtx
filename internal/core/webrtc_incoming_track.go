package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/media"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/stream"
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

	switch strings.ToLower(track.Codec().MimeType) {
	case strings.ToLower(webrtc.MimeTypeAV1):
		t.mediaType = media.TypeVideo
		t.format = &formats.AV1{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP9):
		t.mediaType = media.TypeVideo
		t.format = &formats.VP9{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP8):
		t.mediaType = media.TypeVideo
		t.format = &formats.VP8{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeH264):
		t.mediaType = media.TypeVideo
		t.format = &formats.H264{
			PayloadTyp:        uint8(track.PayloadType()),
			PacketizationMode: 1,
		}

	case strings.ToLower(webrtc.MimeTypeOpus):
		t.mediaType = media.TypeAudio
		t.format = &formats.Opus{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeG722):
		t.mediaType = media.TypeAudio
		t.format = &formats.G722{}

	case strings.ToLower(webrtc.MimeTypePCMU):
		t.mediaType = media.TypeAudio
		t.format = &formats.G711{
			MULaw: true,
		}

	case strings.ToLower(webrtc.MimeTypePCMA):
		t.mediaType = media.TypeAudio
		t.format = &formats.G711{
			MULaw: false,
		}

	default:
		return nil, fmt.Errorf("unsupported codec: %v", track.Codec())
	}

	t.media = &media.Media{
		Type:    t.mediaType,
		Formats: []formats.Format{t.format},
	}

	return t, nil
}

func (t *webRTCIncomingTrack) start(stream *stream.Stream) {
	go func() {
		for {
			pkt, _, err := t.track.ReadRTP()
			if err != nil {
				return
			}

			// sometimes Chrome sends empty RTP packets. ignore them.
			if len(pkt.Payload) == 0 {
				continue
			}

			stream.WriteRTPPacket(t.media, t.format, pkt, time.Now())
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
