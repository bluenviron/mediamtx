package webrtc

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/liberrors"
	"github.com/bluenviron/gortsplib/v4/pkg/rtplossdetector"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"

	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	keyFrameInterval = 2 * time.Second
)

// IncomingTrack is an incoming track.
type IncomingTrack struct {
	track *webrtc.TrackRemote
	log   logger.Writer

	format       format.Format
	lossDetector *rtplossdetector.LossDetector
}

func newIncomingTrack(
	track *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
	writeRTCP func([]rtcp.Packet) error,
	log logger.Writer,
) (*IncomingTrack, error) {
	t := &IncomingTrack{
		track:        track,
		log:          log,
		lossDetector: rtplossdetector.New(),
	}

	isVideo := false

	switch strings.ToLower(track.Codec().MimeType) {
	case strings.ToLower(webrtc.MimeTypeAV1):
		isVideo = true
		t.format = &format.AV1{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP9):
		isVideo = true
		t.format = &format.VP9{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP8):
		isVideo = true
		t.format = &format.VP8{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeH264):
		isVideo = true
		t.format = &format.H264{
			PayloadTyp:        uint8(track.PayloadType()),
			PacketizationMode: 1,
		}

	case strings.ToLower(webrtc.MimeTypeOpus):
		t.format = &format.Opus{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeG722):
		t.format = &format.G722{}

	case strings.ToLower(webrtc.MimeTypePCMU):
		t.format = &format.G711{
			MULaw: true,
		}

	case strings.ToLower(webrtc.MimeTypePCMA):
		t.format = &format.G711{
			MULaw: false,
		}

	default:
		return nil, fmt.Errorf("unsupported codec: %v", track.Codec())
	}

	// read incoming RTCP packets to make interceptors work
	go func() {
		buf := make([]byte, 1500)
		for {
			_, _, err := receiver.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// send period key frame requests
	if isVideo {
		go func() {
			keyframeTicker := time.NewTicker(keyFrameInterval)
			defer keyframeTicker.Stop()

			for range keyframeTicker.C {
				err := writeRTCP([]rtcp.Packet{
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

	return t, nil
}

// Format returns the track format.
func (t *IncomingTrack) Format() format.Format {
	return t.format
}

// ReadRTP reads a RTP packet.
func (t *IncomingTrack) ReadRTP() (*rtp.Packet, error) {
	for {
		pkt, _, err := t.track.ReadRTP()
		if err != nil {
			return nil, err
		}

		lost := t.lossDetector.Process(pkt)
		if lost != 0 {
			t.log.Log(logger.Warn, (liberrors.ErrClientRTPPacketsLost{Lost: lost}).Error())
			// do not return
		}

		// sometimes Chrome sends empty RTP packets. ignore them.
		if len(pkt.Payload) == 0 {
			continue
		}

		return pkt, nil
	}
}
