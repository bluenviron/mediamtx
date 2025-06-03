package webrtc

import (
	"context"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

var webrtcNilLogger = logging.NewDefaultLeveledLoggerForScope("", 0, &nilWriter{})

func gatherCodecs(tracks []*IncomingTrack) []webrtc.RTPCodecParameters {
	codecs := make([]webrtc.RTPCodecParameters, len(tracks))
	for i, track := range tracks {
		codecs[i] = track.Codec()
	}
	return codecs
}

func TestPeerConnectionCloseImmediately(t *testing.T) {
	pc := &PeerConnection{
		LocalRandomUDP:     true,
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

func TestPeerConnectionCandidates(t *testing.T) {
	for _, ca := range []string{
		"udp",
		"stun",
		"udp+stun",
	} {
		t.Run(ca, func(t *testing.T) {
			pc := &PeerConnection{
				IPsFromInterfaces:     true,
				IPsFromInterfacesList: []string{"lo"},
				HandshakeTimeout:      conf.Duration(10 * time.Second),
				TrackGatherTimeout:    conf.Duration(2 * time.Second),
				Log:                   test.NilLogger,
			}

			if ca == "udp" || ca == "udp+stun" {
				pc.LocalRandomUDP = true
			}
			if ca == "stun" || ca == "udp+stun" {
				pc.ICEServers = []webrtc.ICEServer{{
					URLs: []string{"stun:stun.l.google.com:19302"},
				}}
			}

			err := pc.Start()
			require.NoError(t, err)
			defer pc.Close()

			_, err = pc.CreatePartialOffer()
			require.NoError(t, err)

			// convert partial offer into full offer
			err = pc.waitGatheringDone(context.Background())
			require.NoError(t, err)

			offer := pc.wr.LocalDescription()

			if ca == "udp" || ca == "udp+stun" {
				require.Equal(t, 2, strings.Count(offer.SDP, "typ host"))
			}
			if ca == "stun" || ca == "udp+stun" {
				require.Equal(t, 2, strings.Count(offer.SDP, "typ srflx"))
			}
		})
	}
}

func TestPeerConnectionConnectivity(t *testing.T) {
	for _, mode := range []string{
		"passive udp",
		"passive tcp",
		"active udp",
		"active udp + stun",
	} {
		for _, ip := range []string{
			"from interfaces",
			"additional hosts",
		} {
			// LocalRandomUDP doesn't work with AdditionalHosts
			// we do not care since currently we are not using them together
			if mode == "active udp" && ip == "additional hosts" {
				continue
			}

			t.Run(mode+"_"+ip, func(t *testing.T) {
				var iceServers []webrtc.ICEServer

				if mode == "active udp + stun" {
					iceServers = []webrtc.ICEServer{{
						URLs: []string{"stun:stun.l.google.com:19302"},
					}}
				}

				clientPC := &PeerConnection{
					LocalRandomUDP:        (mode == "passive udp" || mode == "active udp"),
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

				switch mode {
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
					LocalRandomUDP:     (mode == "active udp"),
					ICEUDPMux:          udpMux,
					ICETCPMux:          tcpMux,
					ICEServers:         iceServers,
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

				if ip == "from interfaces" {
					serverPC.IPsFromInterfaces = true
					serverPC.IPsFromInterfacesList = []string{"lo"}
				} else {
					serverPC.AdditionalHosts = []string{"127.0.0.2"}
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

				require.Equal(t, 2, strings.Count(answer.SDP, "a=candidate:"))

				err = clientPC.SetAnswer(answer)
				require.NoError(t, err)

				err = serverPC.WaitUntilConnected(context.Background())
				require.NoError(t, err)

				switch mode {
				case "passive udp":
					if ip == "from interfaces" {
						require.Regexp(t, "^host/udp/127\\.0\\.0\\.1/4458$", serverPC.LocalCandidate())
					} else {
						require.Regexp(t, "^host/udp/127\\.0\\.0\\.2/4458$", serverPC.LocalCandidate())
					}

				case "passive tcp":
					if ip == "from interfaces" {
						require.Regexp(t, "^host/tcp/127\\.0\\.0\\.1/4458$", serverPC.LocalCandidate())
					} else {
						require.Regexp(t, "^host/tcp/127\\.0\\.0\\.2/4458$", serverPC.LocalCandidate())
					}

				case "active udp":
					require.Regexp(t, "^host/udp/127\\.0\\.0\\.1", serverPC.LocalCandidate())

				case "active udp + stun":
					require.Regexp(t, "^srflx/udp/", serverPC.LocalCandidate())
				}
			})
		}
	}
}

func TestPeerConnectionPublishRead(t *testing.T) {
	pc1 := &PeerConnection{
		LocalRandomUDP:     true,
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
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		Publish:            true,
		OutgoingTracks: []*OutgoingTrack{
			{
				Caps: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeH264,
					ClockRate: 90000,
				},
			},
			{
				Caps: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeOpus,
					ClockRate: 48000,
					Channels:  2,
				},
			},
		},
		Log: test.NilLogger,
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

	err = pc1.WaitUntilConnected(context.Background())
	require.NoError(t, err)

	err = pc2.WaitUntilConnected(context.Background())
	require.NoError(t, err)

	for _, track := range pc2.OutgoingTracks {
		err = track.WriteRTP(&rtp.Packet{
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
	}

	err = pc1.GatherIncomingTracks(context.Background())
	require.NoError(t, err)

	codecs := gatherCodecs(pc1.IncomingTracks())

	sort.Slice(codecs, func(i, j int) bool {
		return codecs[i].PayloadType < codecs[j].PayloadType
	})

	require.Equal(t, []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
				RTCPFeedback: codecs[0].RTCPFeedback,
			},
			PayloadType: 105,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
				RTCPFeedback: codecs[1].RTCPFeedback,
			},
			PayloadType: 111,
		},
	}, codecs)
}

// test that an audio codec is present regardless of the fact that an audio track is.
func TestPeerConnectionFallbackCodecs(t *testing.T) {
	pc1 := &PeerConnection{
		LocalRandomUDP:     true,
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
		LocalRandomUDP:     true,
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
