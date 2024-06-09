package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestPeerConnectionCloseImmediately(t *testing.T) {
	pc := &PeerConnection{
		HandshakeTimeout:   conf.StringDuration(10 * time.Second),
		TrackGatherTimeout: conf.StringDuration(2 * time.Second),
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		Publish:            false,
		Log:                test.NilLogger,
	}
	err := pc.Start()
	require.NoError(t, err)
	defer pc.Close()

	_, err = pc.CreatePartialOffer()
	require.NoError(t, err)

	// wait for ICE candidates to be generated
	time.Sleep(500 * time.Millisecond)

	pc.Close()
}

func TestPeerConnectionPublishRead(t *testing.T) {
	for _, ca := range []struct {
		name string
		in   format.Format
		out  format.Format
	}{
		{
			"av1",
			&format.AV1{
				PayloadTyp: 96,
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
			&format.VP9{
				PayloadTyp: 96,
			},
		},
		{
			"vp8",
			&format.VP8{
				PayloadTyp: 96,
			},
			&format.VP8{
				PayloadTyp: 96,
			},
		},
		{
			"h264",
			test.FormatH264,
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
			&format.Opus{
				PayloadTyp:   96,
				ChannelCount: 1,
			},
		},
		{
			"g722",
			&format.G722{},
			&format.G722{},
		},
		{
			"g711 pcma stereo",
			&format.G711{
				PayloadTyp:   96,
				SampleRate:   8000,
				ChannelCount: 2,
			},
			&format.G711{
				PayloadTyp:   119,
				SampleRate:   8000,
				ChannelCount: 2,
			},
		},
		{
			"g711 pcmu stereo",
			&format.G711{
				MULaw:        true,
				PayloadTyp:   96,
				SampleRate:   8000,
				ChannelCount: 2,
			},
			&format.G711{
				MULaw:        true,
				PayloadTyp:   118,
				SampleRate:   8000,
				ChannelCount: 2,
			},
		},
		{
			"g711 pcma mono",
			&format.G711{
				PayloadTyp:   8,
				SampleRate:   8000,
				ChannelCount: 1,
			},
			&format.G711{
				PayloadTyp:   8,
				SampleRate:   8000,
				ChannelCount: 1,
			},
		},
		{
			"g711 pcmu mono",
			&format.G711{
				MULaw:        true,
				PayloadTyp:   0,
				SampleRate:   8000,
				ChannelCount: 1,
			},
			&format.G711{
				MULaw:        true,
				PayloadTyp:   0,
				SampleRate:   8000,
				ChannelCount: 1,
			},
		},
		{
			"l16 8000 stereo",
			&format.LPCM{
				PayloadTyp:   96,
				BitDepth:     16,
				SampleRate:   8000,
				ChannelCount: 2,
			},
			&format.LPCM{
				PayloadTyp:   96,
				BitDepth:     16,
				SampleRate:   8000,
				ChannelCount: 2,
			},
		},
		{
			"l16 16000 stereo",
			&format.LPCM{
				PayloadTyp:   96,
				BitDepth:     16,
				SampleRate:   16000,
				ChannelCount: 2,
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
			&format.LPCM{
				PayloadTyp:   96,
				BitDepth:     16,
				SampleRate:   48000,
				ChannelCount: 2,
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			pc1 := &PeerConnection{
				HandshakeTimeout:   conf.StringDuration(10 * time.Second),
				TrackGatherTimeout: conf.StringDuration(2 * time.Second),
				LocalRandomUDP:     true,
				IPsFromInterfaces:  true,
				Publish:            true,
				OutgoingTracks: []*OutgoingTrack{{
					Format: ca.in,
				}},
				Log: test.NilLogger,
			}
			err := pc1.Start()
			require.NoError(t, err)
			defer pc1.Close()

			pc2 := &PeerConnection{
				HandshakeTimeout:   conf.StringDuration(10 * time.Second),
				TrackGatherTimeout: conf.StringDuration(2 * time.Second),
				LocalRandomUDP:     true,
				IPsFromInterfaces:  true,
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

			inc, err := pc2.GatherIncomingTracks(context.Background())
			require.NoError(t, err)

			require.Equal(t, ca.out, inc[0].Format())
		})
	}
}
