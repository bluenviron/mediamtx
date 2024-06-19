package webrtc

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
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
				SDPFmtpLine: "profile-id=0",
			},
			PayloadType: 96,
		}, nil

	case *format.VP8:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 96,
		}, nil

	case *format.H265:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeH265,
				ClockRate: 90000,
			},
			PayloadType: 96,
		}, nil

	case *format.H264:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			},
			PayloadType: 96,
		}, nil

	case *format.Opus:
		switch forma.ChannelCount {
		case 1, 2:
			return webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeOpus,
					ClockRate: 48000,
					Channels:  2,
					SDPFmtpLine: func() string {
						s := "minptime=10;useinbandfec=1"
						if forma.ChannelCount == 2 {
							s += ";stereo=1;sprop-stereo=1"
						}
						return s
					}(),
				},
				PayloadType: 96,
			}, nil

		case 3, 4, 5, 6, 7, 8:
			return webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:    mimeTypeMultiopus,
					ClockRate:   48000,
					Channels:    uint16(forma.ChannelCount),
					SDPFmtpLine: multichannelOpusSDP[forma.ChannelCount],
				},
				PayloadType: 96,
			}, nil

		default:
			return webrtc.RTPCodecParameters{}, fmt.Errorf("unsupported channel count: %d", forma.ChannelCount)
		}

	case *format.G722:
		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeG722,
				ClockRate: 8000,
			},
			PayloadType: 9,
		}, nil

	case *format.G711:
		// These are the sample rates and channels supported by Chrome.
		// Different sample rates and channels can be streamed too but we don't want compatibility issues.
		// https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/pcm16b/audio_decoder_pcm16b.cc#23
		if forma.ClockRate() != 8000 && forma.ClockRate() != 16000 &&
			forma.ClockRate() != 32000 && forma.ClockRate() != 48000 {
			return webrtc.RTPCodecParameters{}, fmt.Errorf("unsupported clock rate: %d", forma.ClockRate())
		}
		if forma.ChannelCount != 1 && forma.ChannelCount != 2 {
			return webrtc.RTPCodecParameters{}, fmt.Errorf("unsupported channel count: %d", forma.ChannelCount)
		}

		if forma.SampleRate == 8000 {
			if forma.MULaw {
				if forma.ChannelCount != 1 {
					return webrtc.RTPCodecParameters{
						RTPCodecCapability: webrtc.RTPCodecCapability{
							MimeType:  webrtc.MimeTypePCMU,
							ClockRate: uint32(forma.SampleRate),
							Channels:  uint16(forma.ChannelCount),
						},
						PayloadType: 96,
					}, nil
				}

				return webrtc.RTPCodecParameters{
					RTPCodecCapability: webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMU,
						ClockRate: 8000,
					},
					PayloadType: 0,
				}, nil
			}

			if forma.ChannelCount != 1 {
				return webrtc.RTPCodecParameters{
					RTPCodecCapability: webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMA,
						ClockRate: uint32(forma.SampleRate),
						Channels:  uint16(forma.ChannelCount),
					},
					PayloadType: 96,
				}, nil
			}

			return webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypePCMA,
					ClockRate: 8000,
				},
				PayloadType: 8,
			}, nil
		}

		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  mimeTypeL16,
				ClockRate: uint32(forma.ClockRate()),
				Channels:  uint16(forma.ChannelCount),
			},
			PayloadType: 96,
		}, nil

	case *format.LPCM:
		if forma.BitDepth != 16 {
			return webrtc.RTPCodecParameters{}, fmt.Errorf("unsupported LPCM bit depth: %d", forma.BitDepth)
		}

		// These are the sample rates and channels supported by Chrome.
		// Different sample rates and channels can be streamed too but we don't want compatibility issues.
		// https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/pcm16b/audio_decoder_pcm16b.cc#23
		if forma.ClockRate() != 8000 && forma.ClockRate() != 16000 &&
			forma.ClockRate() != 32000 && forma.ClockRate() != 48000 {
			return webrtc.RTPCodecParameters{}, fmt.Errorf("unsupported clock rate: %d", forma.ClockRate())
		}
		if forma.ChannelCount != 1 && forma.ChannelCount != 2 {
			return webrtc.RTPCodecParameters{}, fmt.Errorf("unsupported channel count: %d", forma.ChannelCount)
		}

		return webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  mimeTypeL16,
				ClockRate: uint32(forma.ClockRate()),
				Channels:  uint16(forma.ChannelCount),
			},
			PayloadType: 96,
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
		*format.H265,
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
