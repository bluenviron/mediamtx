package whip

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func whipOffer(body []byte) *pwebrtc.SessionDescription {
	return &pwebrtc.SessionDescription{
		Type: pwebrtc.SDPTypeOffer,
		SDP:  string(body),
	}
}

func gatherCodecs(tracks []*webrtc.IncomingTrack) []pwebrtc.RTPCodecParameters {
	codecs := make([]pwebrtc.RTPCodecParameters, len(tracks))
	for i, track := range tracks {
		codecs[i] = track.Codec()
	}
	return codecs
}

func TestClientRead(t *testing.T) {
	for _, ca := range []string{
		"audio",
		"video+audio",
	} {
		t.Run(ca, func(t *testing.T) {
			var outgoingTracks []*webrtc.OutgoingTrack

			switch ca {
			case "audio":
				outgoingTracks = []*webrtc.OutgoingTrack{{
					Caps: pwebrtc.RTPCodecCapability{
						MimeType:  "audio/opus",
						ClockRate: 48000,
						Channels:  2,
					},
				}}

			case "video+audio":
				outgoingTracks = []*webrtc.OutgoingTrack{
					{
						Caps: pwebrtc.RTPCodecCapability{
							MimeType:  "video/H264",
							ClockRate: 90000,
						},
					},
					{
						Caps: pwebrtc.RTPCodecCapability{
							MimeType:  "audio/opus",
							ClockRate: 48000,
							Channels:  2,
						},
					},
				}
			}

			pc := &webrtc.PeerConnection{
				LocalRandomUDP:     true,
				IPsFromInterfaces:  true,
				Publish:            true,
				HandshakeTimeout:   conf.Duration(10 * time.Second),
				TrackGatherTimeout: conf.Duration(2 * time.Second),
				STUNGatherTimeout:  conf.Duration(5 * time.Second),
				OutgoingTracks:     outgoingTracks,
				Log:                test.NilLogger,
			}
			err := pc.Start()
			require.NoError(t, err)
			defer pc.Close()

			state := 0

			httpServ := &http.Server{
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch state {
					case 0:
						require.Equal(t, http.MethodOptions, r.Method)
						require.Equal(t, "/my/resource", r.URL.Path)

						w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH")
						w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
						w.WriteHeader(http.StatusNoContent)

					case 1:
						require.Equal(t, http.MethodPost, r.Method)
						require.Equal(t, "/my/resource", r.URL.Path)
						require.Equal(t, "application/sdp", r.Header.Get("Content-Type"))

						body, err2 := io.ReadAll(r.Body)
						require.NoError(t, err2)
						offer := whipOffer(body)

						answer, err2 := pc.CreateFullAnswer(context.Background(), offer)
						require.NoError(t, err2)

						w.Header().Set("Content-Type", "application/sdp")
						w.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
						w.Header().Set("ETag", "test_etag")
						w.Header().Set("Location", "/my/resource/sessionid")
						w.WriteHeader(http.StatusCreated)
						w.Write([]byte(answer.SDP))

						go func() {
							err3 := pc.WaitUntilConnected(context.Background())
							require.NoError(t, err3)

							for _, track := range outgoingTracks {
								err3 = track.WriteRTP(&rtp.Packet{
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
								require.NoError(t, err3)
							}
						}()

					default:
						require.Equal(t, "/my/resource/sessionid", r.URL.Path)

						switch r.Method {
						case http.MethodPatch:
							w.WriteHeader(http.StatusNoContent)

						case http.MethodDelete:
							w.WriteHeader(http.StatusOK)

						default:
							t.Errorf("should not happen")
						}
					}
					state++
				}),
			}

			ln, err := net.Listen("tcp", "localhost:9005")
			require.NoError(t, err)

			go httpServ.Serve(ln)
			defer httpServ.Shutdown(context.Background())

			u, err := url.Parse("http://localhost:9005/my/resource")
			require.NoError(t, err)

			cl := &Client{
				URL:        u,
				HTTPClient: &http.Client{},
				Log:        test.NilLogger,
			}
			err = cl.Initialize(context.Background())
			require.NoError(t, err)
			defer cl.Close() //nolint:errcheck

			codecs := gatherCodecs(cl.IncomingTracks())

			switch ca {
			case "audio":
				require.Equal(t, []pwebrtc.RTPCodecParameters{
					{
						RTPCodecCapability: pwebrtc.RTPCodecCapability{
							MimeType:    pwebrtc.MimeTypeOpus,
							ClockRate:   48000,
							Channels:    2,
							SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
							RTCPFeedback: []pwebrtc.RTCPFeedback{{
								Type: "transport-cc",
							}},
						},
						PayloadType: 111,
					},
				}, codecs)

			case "video+audio":
				sort.Slice(codecs, func(i, j int) bool {
					return codecs[i].PayloadType < codecs[j].PayloadType
				})

				require.Equal(t, []pwebrtc.RTPCodecParameters{
					{
						RTPCodecCapability: pwebrtc.RTPCodecCapability{
							MimeType:     pwebrtc.MimeTypeH264,
							ClockRate:    90000,
							SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
							RTCPFeedback: codecs[0].RTCPFeedback,
						},
						PayloadType: 105,
					},
					{
						RTPCodecCapability: pwebrtc.RTPCodecCapability{
							MimeType:     pwebrtc.MimeTypeOpus,
							ClockRate:    48000,
							Channels:     2,
							SDPFmtpLine:  "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
							RTCPFeedback: codecs[1].RTCPFeedback,
						},
						PayloadType: 111,
					},
				}, codecs)
			}

			recv := make([]chan struct{}, len(outgoingTracks))
			for i := range outgoingTracks {
				recv[i] = make(chan struct{})
			}

			for i, track := range cl.IncomingTracks() {
				ci := i
				track.OnPacketRTP = func(_ *rtp.Packet, _ time.Time) {
					close(recv[ci])
				}
			}

			cl.StartReading()

			for _, rv := range recv {
				<-rv
			}
		})
	}
}

func TestClientPublish(t *testing.T) {
	for _, ca := range []string{"audio", "video+audio"} {
		t.Run(ca, func(t *testing.T) {
			pc := &webrtc.PeerConnection{
				LocalRandomUDP:     true,
				IPsFromInterfaces:  true,
				HandshakeTimeout:   conf.Duration(10 * time.Second),
				TrackGatherTimeout: conf.Duration(2 * time.Second),
				STUNGatherTimeout:  conf.Duration(5 * time.Second),
				Log:                test.NilLogger,
			}
			err := pc.Start()
			require.NoError(t, err)
			defer pc.Close()

			state := 0
			var recv []chan struct{}

			httpServ := &http.Server{
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch state {
					case 0:
						require.Equal(t, http.MethodOptions, r.Method)
						require.Equal(t, "/my/resource", r.URL.Path)

						w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH")
						w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
						w.WriteHeader(http.StatusNoContent)

					case 1:
						require.Equal(t, http.MethodPost, r.Method)
						require.Equal(t, "/my/resource", r.URL.Path)
						require.Equal(t, "application/sdp", r.Header.Get("Content-Type"))

						body, err2 := io.ReadAll(r.Body)
						require.NoError(t, err2)
						offer := whipOffer(body)

						answer, err2 := pc.CreateFullAnswer(context.Background(), offer)
						require.NoError(t, err2)

						w.Header().Set("Content-Type", "application/sdp")
						w.Header().Set("Accept-Patch", "application/trickle-ice-sdpfrag")
						w.Header().Set("ETag", "test_etag")
						w.Header().Set("Location", "/my/resource/sessionid")
						w.WriteHeader(http.StatusCreated)
						w.Write([]byte(answer.SDP))

						go func() {
							err3 := pc.WaitUntilConnected(context.Background())
							require.NoError(t, err3)

							err3 = pc.GatherIncomingTracks(context.Background())
							require.NoError(t, err3)

							codecs := gatherCodecs(pc.IncomingTracks())

							switch ca {
							case "audio":
								require.Equal(t, []pwebrtc.RTPCodecParameters{
									{
										RTPCodecCapability: pwebrtc.RTPCodecCapability{
											MimeType:     pwebrtc.MimeTypeOpus,
											ClockRate:    48000,
											Channels:     2,
											SDPFmtpLine:  "",
											RTCPFeedback: codecs[0].RTCPFeedback,
										},
										PayloadType: 96,
									},
								}, codecs)

							case "video+audio":
								sort.Slice(codecs, func(i, j int) bool {
									return codecs[i].PayloadType < codecs[j].PayloadType
								})

								require.Equal(t, []pwebrtc.RTPCodecParameters{
									{
										RTPCodecCapability: pwebrtc.RTPCodecCapability{
											MimeType:     pwebrtc.MimeTypeH264,
											ClockRate:    90000,
											RTCPFeedback: codecs[0].RTCPFeedback,
										},
										PayloadType: 96,
									},
									{
										RTPCodecCapability: pwebrtc.RTPCodecCapability{
											MimeType:     pwebrtc.MimeTypeOpus,
											ClockRate:    48000,
											Channels:     2,
											SDPFmtpLine:  "",
											RTCPFeedback: codecs[1].RTCPFeedback,
										},
										PayloadType: 97,
									},
								}, codecs)
							}

							for i, track := range pc.IncomingTracks() {
								ci := i
								track.OnPacketRTP = func(_ *rtp.Packet, _ time.Time) {
									close(recv[ci])
								}
							}

							pc.StartReading()
						}()

					default:
						require.Equal(t, "/my/resource/sessionid", r.URL.Path)

						switch r.Method {
						case http.MethodPatch:
							w.WriteHeader(http.StatusNoContent)

						case http.MethodDelete:
							w.WriteHeader(http.StatusOK)

						default:
							t.Errorf("should not happen")
						}
					}
					state++
				}),
			}

			ln, err := net.Listen("tcp", "localhost:9005")
			require.NoError(t, err)

			go httpServ.Serve(ln)
			defer httpServ.Shutdown(context.Background())

			u, err := url.Parse("http://localhost:9005/my/resource")
			require.NoError(t, err)

			var outgoingTracks []*webrtc.OutgoingTrack

			switch ca {
			case "audio":
				outgoingTracks = []*webrtc.OutgoingTrack{{
					Caps: pwebrtc.RTPCodecCapability{
						MimeType:  "audio/opus",
						ClockRate: 48000,
						Channels:  2,
					},
				}}

			case "video+audio":
				outgoingTracks = []*webrtc.OutgoingTrack{
					{
						Caps: pwebrtc.RTPCodecCapability{
							MimeType:  "video/H264",
							ClockRate: 90000,
						},
					},
					{
						Caps: pwebrtc.RTPCodecCapability{
							MimeType:  "audio/opus",
							ClockRate: 48000,
							Channels:  2,
						},
					},
				}
			}

			recv = make([]chan struct{}, len(outgoingTracks))
			for i := range outgoingTracks {
				recv[i] = make(chan struct{})
			}

			cl := &Client{
				URL:            u,
				Publish:        true,
				OutgoingTracks: outgoingTracks,
				HTTPClient:     &http.Client{},
				Log:            test.NilLogger,
			}
			err = cl.Initialize(context.Background())
			require.NoError(t, err)
			defer cl.Close() //nolint:errcheck

			for _, track := range cl.OutgoingTracks {
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

			for _, rv := range recv {
				<-rv
			}
		})
	}
}
