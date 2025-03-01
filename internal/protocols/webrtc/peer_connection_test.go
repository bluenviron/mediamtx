package webrtc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

var webrtcNilLogger = logging.NewDefaultLeveledLoggerForScope("", 0, &nilWriter{})

func TestPeerConnectionCloseImmediately(t *testing.T) {
	pc := &PeerConnection{
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		STUNGatherTimeout:  conf.Duration(5 * time.Second),
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

func TestPeerConnectionConnectivity(t *testing.T) {
	for _, ca := range []string{
		"passive udp",
		"passive tcp",
		"active udp",
		"active udp + stun",
	} {
		t.Run(ca, func(t *testing.T) {
			var iceServers []webrtc.ICEServer

			if ca == "active udp + stun" {
				iceServers = []webrtc.ICEServer{{
					URLs: []string{"stun:stun.l.google.com:19302"},
				}}
			}

			clientPC := &PeerConnection{
				IPsFromInterfaces:     true,
				IPsFromInterfacesList: []string{"lo"},
				ICEServers:            iceServers,
				HandshakeTimeout:      conf.Duration(10 * time.Second),
				TrackGatherTimeout:    conf.Duration(2 * time.Second),
				Log:                   test.NilLogger,
			}
			err := clientPC.Start()
			require.NoError(t, err)
			defer clientPC.Close()

			var udpMux ice.UDPMux
			var tcpMux ice.TCPMux

			switch ca {
			case "passive udp":
				var ln net.PacketConn
				ln, err = net.ListenPacket("udp4", ":4458")
				require.NoError(t, err)
				defer ln.Close()
				udpMux = webrtc.NewICEUDPMux(webrtcNilLogger, ln)

			case "passive tcp":
				var ln net.Listener
				ln, err = net.Listen("tcp4", ":4458")
				require.NoError(t, err)
				defer ln.Close()
				tcpMux = webrtc.NewICETCPMux(webrtcNilLogger, ln, 8)
			}

			serverPC := &PeerConnection{
				ICEUDPMux:             udpMux,
				ICETCPMux:             tcpMux,
				IPsFromInterfaces:     true,
				IPsFromInterfacesList: []string{"lo"},
				ICEServers:            iceServers,
				HandshakeTimeout:      conf.Duration(10 * time.Second),
				TrackGatherTimeout:    conf.Duration(2 * time.Second),
				Publish:               true,
				OutgoingTracks: []*OutgoingTrack{{
					Caps: webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypeAV1,
						ClockRate: 90000,
					},
				}},
				Log: test.NilLogger,
			}
			err = serverPC.Start()
			require.NoError(t, err)
			defer serverPC.Close()

			_, err = clientPC.CreatePartialOffer()
			require.NoError(t, err)

			// convert partial offer into full offer
			err = clientPC.waitGatheringDone(context.Background())
			require.NoError(t, err)

			answer, err := serverPC.CreateFullAnswer(context.Background(), clientPC.wr.LocalDescription())
			require.NoError(t, err)

			err = clientPC.SetAnswer(answer)
			require.NoError(t, err)

			err = serverPC.WaitUntilConnected(context.Background())
			require.NoError(t, err)

			switch ca {
			case "passive udp":
				require.Regexp(t, "^host/udp/.*?/4458$", serverPC.LocalCandidate())
				require.Regexp(t, "^host/udp/", clientPC.LocalCandidate())

			case "passive tcp":
				require.Regexp(t, "^host/tcp/.*?/4458$", serverPC.LocalCandidate())
				require.Regexp(t, "^host/tcp/", clientPC.LocalCandidate())

			case "active udp":
				require.Regexp(t, "^host/udp/", serverPC.LocalCandidate())
				require.Regexp(t, "^host/udp/", clientPC.LocalCandidate())

			case "active udp + stun":
				require.Regexp(t, "^srflx/udp/", serverPC.LocalCandidate())
				require.Regexp(t, "^srflx/udp/", clientPC.LocalCandidate())
			}
		})
	}
}

// test that an audio codec is present regardless of the fact that an audio track is.
func TestPeerConnectionFallbackCodecs(t *testing.T) {
	pc1 := &PeerConnection{
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		Publish:            false,
		Log:                test.NilLogger,
	}
	err := pc1.Start()
	require.NoError(t, err)
	defer pc1.Close()

	pc2 := &PeerConnection{
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		Publish:            true,
		OutgoingTracks: []*OutgoingTrack{{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
		}},
		Log: test.NilLogger,
	}
	err = pc2.Start()
	require.NoError(t, err)
	defer pc2.Close()

	offer, err := pc1.CreatePartialOffer()
	require.NoError(t, err)

	answer, err := pc2.CreateFullAnswer(context.Background(), offer)
	require.NoError(t, err)

	var s sdp.SessionDescription
	err = s.Unmarshal([]byte(answer.SDP))
	require.NoError(t, err)

	require.Equal(t, []*sdp.MediaDescription{
		{
			MediaName: sdp.MediaName{
				Media:   "video",
				Port:    sdp.RangedPort{Value: 9},
				Protos:  []string{"UDP", "TLS", "RTP", "SAVPF"},
				Formats: []string{"97"},
			},
			ConnectionInformation: s.MediaDescriptions[0].ConnectionInformation,
			Attributes:            s.MediaDescriptions[0].Attributes,
		},
		{
			MediaName: sdp.MediaName{
				Media:   "audio",
				Port:    sdp.RangedPort{Value: 9},
				Protos:  []string{"UDP", "TLS", "RTP", "SAVPF"},
				Formats: []string{"0"},
			},
			ConnectionInformation: s.MediaDescriptions[1].ConnectionInformation,
			Attributes:            s.MediaDescriptions[1].Attributes,
		},
	}, s.MediaDescriptions)
}
