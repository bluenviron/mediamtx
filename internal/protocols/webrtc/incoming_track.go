package webrtc

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/liberrors"
	"github.com/bluenviron/gortsplib/v4/pkg/rtpreorderer"
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

	format    format.Format
	reorderer *rtpreorderer.Reorderer
	pkts      []*rtp.Packet
}

func newIncomingTrack(
	track *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
	writeRTCP func([]rtcp.Packet) error,
	log logger.Writer,
) (*IncomingTrack, error) {
	t := &IncomingTrack{
		track:     track,
		log:       log,
		reorderer: rtpreorderer.New(),
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
			IsStereo:   strings.Contains(track.Codec().SDPFmtpLine, "stereo=1"),
		}

	case strings.ToLower(webrtc.MimeTypeG722):
		t.format = &format.G722{}

	case strings.ToLower(webrtc.MimeTypePCMU):
		t.format = &format.G711{
			PayloadTyp:   0,
			MULaw:        true,
			SampleRate:   8000,
			ChannelCount: 1,
		}

	case strings.ToLower(webrtc.MimeTypePCMA):
		t.format = &format.G711{
			PayloadTyp:   8,
			MULaw:        false,
			SampleRate:   8000,
			ChannelCount: 1,
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
		if len(t.pkts) != 0 {
			var pkt *rtp.Packet
			pkt, t.pkts = t.pkts[0], t.pkts[1:]

			// sometimes Chrome sends empty RTP packets. ignore them.
			if len(pkt.Payload) == 0 {
				continue
			}

			return pkt, nil
		}

		pkt, _, err := t.track.ReadRTP()
		if err != nil {
			return nil, err
		}

		var lost int
		t.pkts, lost = t.reorderer.Process(pkt)
		if lost != 0 {
			t.log.Log(logger.Warn, (liberrors.ErrClientRTPPacketsLost{Lost: lost}).Error())
			// do not return
		}

		if len(t.pkts) == 0 {
			continue
		}

		pkt, t.pkts = t.pkts[0], t.pkts[1:]

		// sometimes Chrome sends empty RTP packets. ignore them.
		if len(pkt.Payload) == 0 {
			continue
		}

		return pkt, nil
	}
}
