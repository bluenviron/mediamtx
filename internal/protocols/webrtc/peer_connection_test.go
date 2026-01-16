package webrtc

import (
	"net"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
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
		"udp random",
		"udp",
		"tcp",
		"stun",
		"udp+stun",
		"udp random+stun",
	} {
		t.Run(ca, func(t *testing.T) {
			pc2, err := webrtc.NewPeerConnection(webrtc.Configuration{})
			require.NoError(t, err)
			defer pc2.Close() //nolint:errcheck

			track, err := webrtc.NewTrackLocalStaticRTP(
				webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeVP8,
					ClockRate: 90000,
				},
				"video",
				"publisher",
			)
			require.NoError(t, err)

			_, err = pc2.AddTrack(track)
			require.NoError(t, err)

			offer, err := pc2.CreateOffer(nil)
			require.NoError(t, err)

			var udpMux ice.UDPMux
			if ca == "udp" || ca == "udp+stun" {
				var ln net.PacketConn
				ln, err = net.ListenPacket("udp", ":3454")
				require.NoError(t, err)
				defer ln.Close()
				udpMux = webrtc.NewICEUDPMux(webrtcNilLogger, ln)
			}

			var tcpMux *TCPMuxWrapper
			if ca == "tcp" {
				var ln net.Listener
				ln, err = net.Listen("tcp", ":3454")
				require.NoError(t, err)
				defer ln.Close()
				tcpMux = &TCPMuxWrapper{
					Mux: webrtc.NewICETCPMux(webrtcNilLogger, ln, 8),
					Ln:  ln,
				}
			}

			pc := &PeerConnection{
				LocalRandomUDP:        (ca == "udp random" || ca == "udp random+stun"),
				ICEUDPMux:             udpMux,
				ICETCPMux:             tcpMux,
				IPsFromInterfaces:     true,
				IPsFromInterfacesList: []string{"lo"},
				HandshakeTimeout:      conf.Duration(10 * time.Second),
				TrackGatherTimeout:    conf.Duration(2 * time.Second),
				Log:                   test.NilLogger,
			}

			if ca == "stun" || ca == "udp+stun" || ca == "udp random+stun" {
				pc.ICEServers = []webrtc.ICEServer{{
					URLs: []string{"stun:stun.l.google.com:19302"},
				}}
			}

			err = pc.Start()
			require.NoError(t, err)
			defer pc.Close()

			answer, err := pc.CreateFullAnswer(&offer)
			require.NoError(t, err)

			n := len(regexp.MustCompile("(?m)^a=candidate:.+? udp .+? typ host").FindAllString(answer.SDP, -1))
			if ca == "udp" || ca == "udp random" || ca == "udp+stun" || ca == "udp random+stun" {
				require.Equal(t, 2, n)
			} else {
				require.Equal(t, 0, n)
			}

			n = len(regexp.MustCompile("(?m)^a=candidate:.+? tcp .+? typ host tcptype passive").FindAllString(answer.SDP, -1))
			if ca == "tcp" {
				require.Equal(t, 2, n)
			} else {
				require.Equal(t, 0, n)
			}

			n = len(regexp.MustCompile("(?m)^a=candidate:.+? udp .+? typ srflx").FindAllString(answer.SDP, -1))
			if ca == "stun" || ca == "udp+stun" || ca == "udp random+stun" {
				require.Equal(t, 2, n)
			} else {
				require.Equal(t, 0, n)
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
			// we don't care since we are not currently using them together
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
				var tcpMux *TCPMuxWrapper

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
					tcpMux = &TCPMuxWrapper{
						Mux: webrtc.NewICETCPMux(webrtcNilLogger, ln, 8),
						Ln:  ln,
					}
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
					serverPC.AdditionalHosts = []string{"127.0.0.1"}
				}

				err = serverPC.Start()
				require.NoError(t, err)
				defer serverPC.Close()

				offer, err := clientPC.CreatePartialOffer()
				require.NoError(t, err)

				answer, err := serverPC.CreateFullAnswer(offer)
				require.NoError(t, err)

				require.Equal(t, 2, strings.Count(answer.SDP, "a=candidate:"))

				err = clientPC.SetAnswer(answer)
				require.NoError(t, err)

				go func() {
					for {
						select {
						case cd := <-clientPC.NewLocalCandidate():
							err2 := serverPC.AddRemoteCandidate(cd)
							require.NoError(t, err2)

						case <-clientPC.Failed():
							return
						}
					}
				}()

				err = serverPC.WaitUntilConnected()
				require.NoError(t, err)
			})
		}
	}
}

func TestPeerConnectionRead(t *testing.T) {
	pub, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer pub.Close() //nolint:errcheck

	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		"video",
		"publisher")
	require.NoError(t, err)

	videoSender, err := pub.AddTrack(videoTrack)
	require.NoError(t, err)

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
		},
		"audio",
		"publisher")
	require.NoError(t, err)

	audioSender, err := pub.AddTrack(audioTrack)
	require.NoError(t, err)

	reader := &PeerConnection{
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		Publish:            false,
		Log:                test.NilLogger,
	}
	err = reader.Start()
	require.NoError(t, err)
	defer reader.Close()

	offer, err := pub.CreateOffer(nil)
	require.NoError(t, err)

	err = pub.SetLocalDescription(offer)
	require.NoError(t, err)

	answer, err := reader.CreateFullAnswer(&offer)
	require.NoError(t, err)

	err = pub.SetRemoteDescription(*answer)
	require.NoError(t, err)

	err = reader.WaitUntilConnected()
	require.NoError(t, err)

	go func() {
		time.Sleep(100 * time.Millisecond)

		err2 := videoTrack.WriteRTP(&rtp.Packet{
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
		require.NoError(t, err2)

		err2 = audioTrack.WriteRTP(&rtp.Packet{
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
		require.NoError(t, err2)
	}()

	err = reader.GatherIncomingTracks()
	require.NoError(t, err)

	codecs := gatherCodecs(reader.IncomingTracks())

	sort.Slice(codecs, func(i, j int) bool {
		return codecs[i].PayloadType < codecs[j].PayloadType
	})

	require.Equal(t, []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeVP8,
				ClockRate:    90000,
				RTCPFeedback: codecs[0].RTCPFeedback,
			},
			PayloadType: 96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     webrtc.MimeTypeOpus,
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1",
				RTCPFeedback: codecs[1].RTCPFeedback,
			},
			PayloadType: 111,
		},
	}, codecs)

	reader.StartReading()

	pkts, _, err := videoSender.ReadRTCP()
	require.NoError(t, err)
	require.Equal(t, []rtcp.Packet{
		&rtcp.ReceiverReport{
			SSRC: pkts[0].(*rtcp.ReceiverReport).SSRC,
			Reports: []rtcp.ReceptionReport{{
				SSRC:               uint32(videoSender.GetParameters().Encodings[0].SSRC),
				LastSequenceNumber: pkts[0].(*rtcp.ReceiverReport).Reports[0].LastSequenceNumber,
				LastSenderReport:   pkts[0].(*rtcp.ReceiverReport).Reports[0].LastSenderReport,
				Delay:              pkts[0].(*rtcp.ReceiverReport).Reports[0].Delay,
			}},
			ProfileExtensions: []byte{},
		},
	}, pkts)

	pkts, _, err = audioSender.ReadRTCP()
	require.NoError(t, err)
	require.Equal(t, []rtcp.Packet{
		&rtcp.ReceiverReport{
			SSRC: pkts[0].(*rtcp.ReceiverReport).SSRC,
			Reports: []rtcp.ReceptionReport{{
				SSRC:               uint32(audioSender.GetParameters().Encodings[0].SSRC),
				LastSequenceNumber: pkts[0].(*rtcp.ReceiverReport).Reports[0].LastSequenceNumber,
				LastSenderReport:   pkts[0].(*rtcp.ReceiverReport).Reports[0].LastSenderReport,
				Delay:              pkts[0].(*rtcp.ReceiverReport).Reports[0].Delay,
			}},
			ProfileExtensions: []byte{},
		},
	}, pkts)
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

	answer, err := pc2.CreateFullAnswer(offer)
	require.NoError(t, err)

	err = pc1.SetAnswer(answer)
	require.NoError(t, err)

	err = pc1.WaitUntilConnected()
	require.NoError(t, err)

	err = pc2.WaitUntilConnected()
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

	err = pc1.GatherIncomingTracks()
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

	answer, err := pc2.CreateFullAnswer(offer)
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

func TestPeerConnectionPublishDataChannel(t *testing.T) {
	pc1, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer pc1.Close() //nolint:errcheck

	_, err = pc1.CreateDataChannel("", nil)
	require.NoError(t, err)

	dataChanCreated := make(chan struct{})
	dataReceived := make(chan struct{})

	pc1.OnDataChannel(func(dc *webrtc.DataChannel) {
		close(dataChanCreated)

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			require.Equal(t, []byte("test data"), msg.Data)
			close(dataReceived)
		})
	})

	offer, err := pc1.CreateOffer(nil)
	require.NoError(t, err)

	err = pc1.SetLocalDescription(offer)
	require.NoError(t, err)

	pc2 := &PeerConnection{
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		STUNGatherTimeout:  conf.Duration(5 * time.Second),
		Publish:            true,
		OutgoingDataChannels: []*OutgoingDataChannel{
			{
				Label: "test-channel",
			},
		},
		Log: test.NilLogger,
	}
	err = pc2.Start()
	require.NoError(t, err)
	defer pc2.Close()

	answer, err := pc2.CreateFullAnswer(&offer)
	require.NoError(t, err)

	err = pc1.SetRemoteDescription(*answer)
	require.NoError(t, err)

	err = pc2.WaitUntilConnected()
	require.NoError(t, err)

	<-dataChanCreated

	pc2.OutgoingDataChannels[0].Write([]byte("test data"))

	<-dataReceived
}
