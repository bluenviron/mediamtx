package webrtc

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
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

const (
	mimeTypeMultiopus = "audio/multiopus"
	mimeTypeL16       = "audio/L16"
)

var incomingVideoCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeAV1,
			ClockRate:   90000,
			SDPFmtpLine: "profile=1",
		},
		PayloadType: 96,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeAV1,
			ClockRate: 90000,
		},
		PayloadType: 97,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=3",
		},
		PayloadType: 98,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=2",
		},
		PayloadType: 99,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=1",
		},
		PayloadType: 100,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=0",
		},
		PayloadType: 101,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		PayloadType: 102,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeH265,
			ClockRate: 90000,
		},
		PayloadType: 103,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		},
		PayloadType: 104,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 105,
	},
}

var incomingAudioCodecs = []webrtc.RTPCodecParameters{
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    3,
			SDPFmtpLine: "channel_mapping=0,2,1;num_streams=2;coupled_streams=1",
		},
		PayloadType: 112,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    4,
			SDPFmtpLine: "channel_mapping=0,1,2,3;num_streams=2;coupled_streams=2",
		},
		PayloadType: 113,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    5,
			SDPFmtpLine: "channel_mapping=0,4,1,2,3;num_streams=3;coupled_streams=2",
		},
		PayloadType: 114,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    6,
			SDPFmtpLine: "channel_mapping=0,4,1,2,3,5;num_streams=4;coupled_streams=2",
		},
		PayloadType: 115,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    7,
			SDPFmtpLine: "channel_mapping=0,4,1,2,3,5,6;num_streams=4;coupled_streams=4",
		},
		PayloadType: 116,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    mimeTypeMultiopus,
			ClockRate:   48000,
			Channels:    8,
			SDPFmtpLine: "channel_mapping=0,6,1,4,5,2,3,7;num_streams=5;coupled_streams=4",
		},
		PayloadType: 117,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
		},
		PayloadType: 111,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeG722,
			ClockRate: 8000,
		},
		PayloadType: 9,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  2,
		},
		PayloadType: 118,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
			Channels:  2,
		},
		PayloadType: 119,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
		},
		PayloadType: 0,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMA,
			ClockRate: 8000,
		},
		PayloadType: 8,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  mimeTypeL16,
			ClockRate: 8000,
			Channels:  2,
		},
		PayloadType: 120,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  mimeTypeL16,
			ClockRate: 16000,
			Channels:  2,
		},
		PayloadType: 121,
	},
	{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  mimeTypeL16,
			ClockRate: 48000,
			Channels:  2,
		},
		PayloadType: 122,
	},
}

// IncomingTrack is an incoming track.
type IncomingTrack struct {
	track *webrtc.TrackRemote
	log   logger.Writer

	typ       description.MediaType
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

	switch strings.ToLower(track.Codec().MimeType) {
	case strings.ToLower(webrtc.MimeTypeAV1):
		t.typ = description.MediaTypeVideo
		t.format = &format.AV1{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP9):
		t.typ = description.MediaTypeVideo
		t.format = &format.VP9{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeVP8):
		t.typ = description.MediaTypeVideo
		t.format = &format.VP8{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeH265):
		t.typ = description.MediaTypeVideo
		t.format = &format.H265{
			PayloadTyp: uint8(track.PayloadType()),
		}

	case strings.ToLower(webrtc.MimeTypeH264):
		t.typ = description.MediaTypeVideo
		t.format = &format.H264{
			PayloadTyp:        uint8(track.PayloadType()),
			PacketizationMode: 1,
		}

	case strings.ToLower(mimeTypeMultiopus):
		t.typ = description.MediaTypeAudio
		t.format = &format.Opus{
			PayloadTyp:   uint8(track.PayloadType()),
			ChannelCount: int(track.Codec().Channels),
		}

	case strings.ToLower(webrtc.MimeTypeOpus):
		t.typ = description.MediaTypeAudio
		t.format = &format.Opus{
			PayloadTyp: uint8(track.PayloadType()),
			ChannelCount: func() int {
				if strings.Contains(track.Codec().SDPFmtpLine, "stereo=1") {
					return 2
				}
				return 1
			}(),
		}

	case strings.ToLower(webrtc.MimeTypeG722):
		t.typ = description.MediaTypeAudio
		t.format = &format.G722{}

	case strings.ToLower(webrtc.MimeTypePCMU):
		t.typ = description.MediaTypeAudio

		channels := track.Codec().Channels
		if channels == 0 {
			channels = 1
		}

		payloadType := uint8(0)
		if channels > 1 {
			payloadType = 118
		}

		t.format = &format.G711{
			PayloadTyp:   payloadType,
			MULaw:        true,
			SampleRate:   8000,
			ChannelCount: int(channels),
		}

	case strings.ToLower(webrtc.MimeTypePCMA):
		t.typ = description.MediaTypeAudio

		channels := track.Codec().Channels
		if channels == 0 {
			channels = 1
		}

		payloadType := uint8(8)
		if channels > 1 {
			payloadType = 119
		}

		t.format = &format.G711{
			PayloadTyp:   payloadType,
			MULaw:        false,
			SampleRate:   8000,
			ChannelCount: int(channels),
		}

	case strings.ToLower(mimeTypeL16):
		t.typ = description.MediaTypeAudio
		t.format = &format.LPCM{
			PayloadTyp:   uint8(track.PayloadType()),
			BitDepth:     16,
			SampleRate:   int(track.Codec().ClockRate),
			ChannelCount: int(track.Codec().Channels),
		}

	default:
		return nil, fmt.Errorf("unsupported codec: %+v", track.Codec().RTPCodecCapability)
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
	if t.typ == description.MediaTypeVideo {
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
