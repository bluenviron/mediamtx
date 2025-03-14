package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestToStreamNoSupportedCodecs(t *testing.T) {
	pc := &PeerConnection{}
	_, err := ToStream(pc, nil)
	require.Equal(t, errNoSupportedCodecsTo, err)
}

// this is impossible to test since unsupported tracks cause an error
// as they are not included inside incomingVideoCodecs or incomingAudioCodecs
// func TestToStreamSkipUnsupportedTracks(t *testing.T)

var toFromStreamCases = []struct {
	name       string
	in         format.Format
	webrtcCaps webrtc.RTPCodecCapability
	out        format.Format
}{
	{
		"av1",
		&format.AV1{
			PayloadTyp: 96,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "video/AV1",
			ClockRate: 90000,
		},
		&format.AV1{
			PayloadTyp: 96,
		},
	},
	{
		"vp9",
		&format.VP9{
			PayloadTyp: 96,
		},
		webrtc.RTPCodecCapability{
			MimeType:    "video/VP9",
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=0",
		},
		&format.VP9{
			PayloadTyp: 96,
		},
	},
	{
		"vp8",
		&format.VP8{
			PayloadTyp: 96,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
		},
		&format.VP8{
			PayloadTyp: 96,
		},
	},
	{
		"h265",
		&format.H265{
			PayloadTyp: 96,
		},
		webrtc.RTPCodecCapability{
			MimeType:    "video/H265",
			ClockRate:   90000,
			SDPFmtpLine: "level-id=93;profile-id=1;tier-flag=0;tx-mode=SRST",
		},
		&format.H265{
			PayloadTyp: 96,
		},
	},
	{
		"h264",
		test.FormatH264,
		webrtc.RTPCodecCapability{
			MimeType:    "video/H264",
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		&format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		},
	},
	{
		"opus multichannel",
		&format.Opus{
			PayloadTyp:   112,
			ChannelCount: 6,
		},
		webrtc.RTPCodecCapability{
			MimeType:    "audio/multiopus",
			ClockRate:   48000,
			Channels:    6,
			SDPFmtpLine: "channel_mapping=0,4,1,2,3,5;num_streams=4;coupled_streams=2",
		},
		&format.Opus{
			PayloadTyp:   96,
			ChannelCount: 6,
		},
	},
	{
		"opus stereo",
		&format.Opus{
			PayloadTyp:   111,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:    "audio/opus",
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
		},
		&format.Opus{
			PayloadTyp:   96,
			ChannelCount: 2,
		},
	},
	{
		"opus mono",
		&format.Opus{
			PayloadTyp:   111,
			ChannelCount: 1,
		},
		webrtc.RTPCodecCapability{
			MimeType:    "audio/opus",
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		&format.Opus{
			PayloadTyp:   96,
			ChannelCount: 1,
		},
	},
	{
		"g722",
		&format.G722{},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/G722",
			ClockRate: 8000,
		},
		&format.G722{},
	},
	{
		"g711 pcma 8khz mono",
		&format.G711{
			PayloadTyp:   8,
			SampleRate:   8000,
			ChannelCount: 1,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/PCMA",
			ClockRate: 8000,
		},
		&format.G711{
			PayloadTyp:   8,
			SampleRate:   8000,
			ChannelCount: 1,
		},
	},
	{
		"g711 pcmu 8khz mono",
		&format.G711{
			MULaw:        true,
			PayloadTyp:   0,
			SampleRate:   8000,
			ChannelCount: 1,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/PCMU",
			ClockRate: 8000,
		},
		&format.G711{
			MULaw:        true,
			PayloadTyp:   0,
			SampleRate:   8000,
			ChannelCount: 1,
		},
	},
	{
		"g711 pcma 8khz stereo",
		&format.G711{
			PayloadTyp:   96,
			SampleRate:   8000,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/PCMA",
			ClockRate: 8000,
			Channels:  2,
		},
		&format.G711{
			PayloadTyp:   119,
			SampleRate:   8000,
			ChannelCount: 2,
		},
	},
	{
		"g711 pcmu 8khz stereo",
		&format.G711{
			MULaw:        true,
			PayloadTyp:   96,
			SampleRate:   8000,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/PCMU",
			ClockRate: 8000,
			Channels:  2,
		},
		&format.G711{
			MULaw:        true,
			PayloadTyp:   118,
			SampleRate:   8000,
			ChannelCount: 2,
		},
	},
	{
		"g711 pcma 16khz stereo",
		&format.G711{
			PayloadTyp:   96,
			SampleRate:   16000,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/L16",
			ClockRate: 16000,
			Channels:  2,
		},
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   16000,
			ChannelCount: 2,
		},
	},
	{
		"g711 pcmu 16khz stereo",
		&format.G711{
			MULaw:        true,
			PayloadTyp:   96,
			SampleRate:   16000,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/L16",
			ClockRate: 16000,
			Channels:  2,
		},
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   16000,
			ChannelCount: 2,
		},
	},
	{
		"l16 8khz stereo",
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   8000,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/L16",
			ClockRate: 8000,
			Channels:  2,
		},
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   8000,
			ChannelCount: 2,
		},
	},
	{
		"l16 16khz stereo",
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   16000,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/L16",
			ClockRate: 16000,
			Channels:  2,
		},
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   16000,
			ChannelCount: 2,
		},
	},
	{
		"l16 48khz stereo",
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   48000,
			ChannelCount: 2,
		},
		webrtc.RTPCodecCapability{
			MimeType:  "audio/L16",
			ClockRate: 48000,
			Channels:  2,
		},
		&format.LPCM{
			PayloadTyp:   96,
			BitDepth:     16,
			SampleRate:   48000,
			ChannelCount: 2,
		},
	},
}

func TestToStream(t *testing.T) {
	for _, ca := range toFromStreamCases {
		t.Run(ca.name, func(t *testing.T) {
			pc1 := &PeerConnection{
				LocalRandomUDP:     true,
				IPsFromInterfaces:  true,
				HandshakeTimeout:   conf.Duration(10 * time.Second),
				TrackGatherTimeout: conf.Duration(2 * time.Second),
				Publish:            true,
				OutgoingTracks: []*OutgoingTrack{{
					Caps: ca.webrtcCaps,
				}},
				Log: test.NilLogger,
			}
			err := pc1.Start()
			require.NoError(t, err)
			defer pc1.Close()

			pc2 := &PeerConnection{
				LocalRandomUDP:     true,
				IPsFromInterfaces:  true,
				HandshakeTimeout:   conf.Duration(10 * time.Second),
				TrackGatherTimeout: conf.Duration(2 * time.Second),
				Publish:            false,
				Log:                test.NilLogger,
			}
			err = pc2.Start()
			require.NoError(t, err)
			defer pc2.Close()

			offer, err := pc1.CreatePartialOffer()
			require.NoError(t, err)

			answer, err := pc2.CreateFullAnswer(context.Background(), offer)
			require.NoError(t, err)

			err = pc1.SetAnswer(answer)
			require.NoError(t, err)

			go func() {
				for {
					select {
					case cnd := <-pc1.NewLocalCandidate():
						err2 := pc2.AddRemoteCandidate(cnd)
						require.NoError(t, err2)

					case <-pc1.Connected():
						return
					}
				}
			}()

			err = pc1.WaitUntilConnected(context.Background())
			require.NoError(t, err)

			err = pc2.WaitUntilConnected(context.Background())
			require.NoError(t, err)

			err = pc1.OutgoingTracks[0].WriteRTP(&rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    111,
					SequenceNumber: 1123,
					Timestamp:      45343,
					SSRC:           563424,
				},
				Payload: []byte{5, 2},
			})
			require.NoError(t, err)

			err = pc2.GatherIncomingTracks(context.Background())
			require.NoError(t, err)

			var stream *stream.Stream
			medias, err := ToStream(pc2, &stream)
			require.NoError(t, err)
			require.Equal(t, ca.out, medias[0].Formats[0])
		})
	}
}
