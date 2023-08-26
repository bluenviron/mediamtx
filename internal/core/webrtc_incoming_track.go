package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/rtptime"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
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

	mediaType description.MediaType
	format    format.Format
	media     *description.Media
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
		t.mediaType = description.MediaTypeVideo
		t.format = &format.AV1{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP9):
		t.mediaType = description.MediaTypeVideo
		t.format = &format.VP9{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP8):
		t.mediaType = description.MediaTypeVideo
		t.format = &format.VP8{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeH264):
		t.mediaType = description.MediaTypeVideo
		t.format = &format.H264{
			PayloadTyp:        uint8(track.PayloadType()),
			PacketizationMode: 1,
		}

	case strings.ToLower(webrtc.MimeTypeOpus):
		t.mediaType = description.MediaTypeAudio
		t.format = &format.Opus{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeG722):
		t.mediaType = description.MediaTypeAudio
		t.format = &format.G722{}

	case strings.ToLower(webrtc.MimeTypePCMU):
		t.mediaType = description.MediaTypeAudio
		t.format = &format.G711{
			MULaw: true,
		}

	case strings.ToLower(webrtc.MimeTypePCMA):
		t.mediaType = description.MediaTypeAudio
		t.format = &format.G711{
			MULaw: false,
		}

	default:
		return nil, fmt.Errorf("unsupported codec: %v", track.Codec())
	}

	t.media = &description.Media{
		Type:    t.mediaType,
		Formats: []format.Format{t.format},
	}

	return t, nil
}

type webrtcTrackWrapper struct {
	clockRate int
}

func (w webrtcTrackWrapper) ClockRate() int {
	return w.clockRate
}

func (webrtcTrackWrapper) PTSEqualsDTS(*rtp.Packet) bool {
	return true
}

func (t *webRTCIncomingTrack) start(stream *stream.Stream, timeDecoder *rtptime.GlobalDecoder) {
	trackWrapper := &webrtcTrackWrapper{clockRate: int(t.track.Codec().ClockRate)}

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

			pts, ok := timeDecoder.Decode(trackWrapper, pkt)
			if !ok {
				continue
			}

			stream.WriteRTPPacket(t.media, t.format, pkt, time.Now(), pts)
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

	if t.mediaType == description.MediaTypeVideo {
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
