package core

import (
	"bytes"
	"crypto/tls"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/auth"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/conn"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/url"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type testServer struct {
	onDescribe func(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error)
	onSetup    func(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error)
	onPlay     func(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error)
}

func (sh *testServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	return sh.onDescribe(ctx)
}

func (sh *testServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return sh.onSetup(ctx)
}

func (sh *testServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return sh.onPlay(ctx)
}

func TestRTSPSource(t *testing.T) {
	for _, source := range []string{
		"udp",
		"tcp",
		"tls",
	} {
		t.Run(source, func(t *testing.T) {
			track := &gortsplib.TrackH264{
				PayloadType:       96,
				SPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PPS:               []byte{0x01, 0x02, 0x03, 0x04},
				PacketizationMode: 1,
			}

			stream := gortsplib.NewServerStream(gortsplib.Tracks{track})

			var authValidator *auth.Validator

			s := gortsplib.Server{
				Handler: &testServer{
					onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx,
					) (*base.Response, *gortsplib.ServerStream, error) {
						if authValidator == nil {
							authValidator = auth.NewValidator("testuser", "testpass", nil)
						}

						err := authValidator.ValidateRequest(ctx.Request)
						if err != nil {
							return &base.Response{
								StatusCode: base.StatusUnauthorized,
								Header: base.Header{
									"WWW-Authenticate": authValidator.Header(),
								},
							}, nil, nil
						}

						return &base.Response{
							StatusCode: base.StatusOK,
						}, stream, nil
					},
					onSetup: func(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, stream, nil
					},
					onPlay: func(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
						go func() {
							time.Sleep(1 * time.Second)
							stream.WritePacketRTP(0, &rtp.Packet{
								Header: rtp.Header{
									Version:        0x02,
									PayloadType:    97,
									SequenceNumber: 57899,
									Timestamp:      345234345,
									SSRC:           978651231,
									Marker:         true,
								},
								Payload: []byte{0x01, 0x02, 0x03, 0x04},
							})
						}()

						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
				},
				RTSPAddress: "127.0.0.1:8555",
			}

			switch source {
			case "udp":
				s.UDPRTPAddress = "127.0.0.1:8002"
				s.UDPRTCPAddress = "127.0.0.1:8003"

			case "tls":
				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				cert, err := tls.LoadX509KeyPair(serverCertFpath, serverKeyFpath)
				require.NoError(t, err)

				s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Wait()
			defer s.Close()

			if source == "udp" || source == "tcp" {
				p, ok := newInstance("paths:\n" +
					"  proxied:\n" +
					"    source: rtsp://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceProtocol: " + source + "\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p.Close()
			} else {
				p, ok := newInstance("paths:\n" +
					"  proxied:\n" +
					"    source: rtsps://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceFingerprint: 33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p.Close()
			}

			received := make(chan struct{})

			c := gortsplib.Client{
				OnPacketRTP: func(ctx *gortsplib.ClientOnPacketRTPCtx) {
					require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, ctx.Packet.Payload)
					close(received)
				},
			}

			u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			tracks, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAndPlay(tracks, baseURL)
			require.NoError(t, err)

			<-received
		})
	}
}

func TestRTSPSourceNoPassword(t *testing.T) {
	track := &gortsplib.TrackH264{
		PayloadType:       96,
		SPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PacketizationMode: 1,
	}

	stream := gortsplib.NewServerStream(gortsplib.Tracks{track})

	var authValidator *auth.Validator
	done := make(chan struct{})

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				if authValidator == nil {
					authValidator = auth.NewValidator("testuser", "", nil)
				}

				err := authValidator.ValidateRequest(ctx.Request)
				if err != nil {
					return &base.Response{
						StatusCode: base.StatusUnauthorized,
						Header: base.Header{
							"WWW-Authenticate": authValidator.Header(),
						},
					}, nil, nil
				}

				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onSetup: func(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				close(done)
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "127.0.0.1:8555",
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Wait()
	defer s.Close()

	p, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://testuser:@127.0.0.1:8555/teststream\n" +
		"    sourceProtocol: tcp\n")
	require.Equal(t, true, ok)
	defer p.Close()

	<-done
}

func TestRTSPSourceDynamicH264Params(t *testing.T) {
	track := &gortsplib.TrackH264{
		PayloadType:       96,
		PacketizationMode: 1,
	}
	stream := gortsplib.NewServerStream(gortsplib.Tracks{track})
	defer stream.Close()

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onSetup: func(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "127.0.0.1:8555",
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Wait()
	defer s.Close()

	p, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://127.0.0.1:8555/teststream\n")
	require.Equal(t, true, ok)
	defer p.Close()

	time.Sleep(1 * time.Second)

	enc := track.CreateEncoder()

	pkts, err := enc.Encode([][]byte{{7, 1, 2, 3}}, 0) // SPS
	require.NoError(t, err)
	stream.WritePacketRTP(0, pkts[0])

	pkts, err = enc.Encode([][]byte{{8}}, 0) // PPS
	require.NoError(t, err)
	stream.WritePacketRTP(0, pkts[0])

	time.Sleep(500 * time.Millisecond)

	func() {
		c := gortsplib.Client{}

		u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
		require.NoError(t, err)

		err = c.Start(u.Scheme, u.Host)
		require.NoError(t, err)
		defer c.Close()

		tracks, _, _, err := c.Describe(u)
		require.NoError(t, err)

		h264Track, ok := tracks[0].(*gortsplib.TrackH264)
		require.Equal(t, true, ok)
		require.Equal(t, []byte{7, 1, 2, 3}, h264Track.SafeSPS())
		require.Equal(t, []byte{8}, h264Track.SafePPS())
	}()

	pkts, err = enc.Encode([][]byte{{7, 4, 5, 6}}, 0) // SPS
	require.NoError(t, err)
	stream.WritePacketRTP(0, pkts[0])

	pkts, err = enc.Encode([][]byte{{8, 1}}, 0) // PPS
	require.NoError(t, err)
	stream.WritePacketRTP(0, pkts[0])

	time.Sleep(500 * time.Millisecond)

	func() {
		c := gortsplib.Client{}

		u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
		require.NoError(t, err)

		err = c.Start(u.Scheme, u.Host)
		require.NoError(t, err)
		defer c.Close()

		tracks, _, _, err := c.Describe(u)
		require.NoError(t, err)

		h264Track, ok := tracks[0].(*gortsplib.TrackH264)
		require.Equal(t, true, ok)
		require.Equal(t, []byte{7, 4, 5, 6}, h264Track.SafeSPS())
		require.Equal(t, []byte{8, 1}, h264Track.SafePPS())
	}()
}

func TestRTSPSourceRemovePadding(t *testing.T) {
	stream := gortsplib.NewServerStream(gortsplib.Tracks{&gortsplib.TrackH264{
		PayloadType:       96,
		PacketizationMode: 1,
	}})
	defer stream.Close()

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onSetup: func(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "127.0.0.1:8555",
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Wait()
	defer s.Close()

	p, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://127.0.0.1:8555/teststream\n")
	require.Equal(t, true, ok)
	defer p.Close()

	time.Sleep(1 * time.Second)

	packetRecv := make(chan struct{})

	c := gortsplib.Client{
		OnPacketRTP: func(ctx *gortsplib.ClientOnPacketRTPCtx) {
			require.Equal(t, &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					PayloadType:    96,
					SequenceNumber: 123,
					Timestamp:      45343,
					SSRC:           563423,
					CSRC:           []uint32{},
				},
				Payload: []byte{0x01, 0x02, 0x03, 0x04},
			}, ctx.Packet)
			close(packetRecv)
		},
	}

	u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	tracks, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	err = c.SetupAndPlay(tracks, baseURL)
	require.NoError(t, err)

	stream.WritePacketRTP(0, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
			Padding:        true,
		},
		Payload:     []byte{0x01, 0x02, 0x03, 0x04},
		PaddingSize: 20,
	})

	<-packetRecv
}

func TestRTSPSourceOversizedPackets(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:8555")
	require.NoError(t, err)
	defer l.Close()

	connected := make(chan struct{})

	serverDone := make(chan struct{})
	defer func() { <-serverDone }()
	go func() {
		defer close(serverDone)

		nconn, err := l.Accept()
		require.NoError(t, err)
		defer nconn.Close()
		conn := conn.NewConn(nconn)

		req, err := conn.ReadRequest()
		require.NoError(t, err)
		require.Equal(t, base.Options, req.Method)

		err = conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Public": base.HeaderValue{strings.Join([]string{
					string(base.Describe),
					string(base.Setup),
					string(base.Play),
				}, ", ")},
			},
		})
		require.NoError(t, err)

		req, err = conn.ReadRequest()
		require.NoError(t, err)
		require.Equal(t, base.Describe, req.Method)

		tracks := gortsplib.Tracks{&gortsplib.TrackH264{
			PayloadType:       96,
			SPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PPS:               []byte{0x01, 0x02, 0x03, 0x04},
			PacketizationMode: 1,
		}}

		err = conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Content-Type": base.HeaderValue{"application/sdp"},
			},
			Body: tracks.Marshal(false),
		})
		require.NoError(t, err)

		req, err = conn.ReadRequest()
		require.NoError(t, err)
		require.Equal(t, base.Setup, req.Method)

		var inTH headers.Transport
		err = inTH.Unmarshal(req.Header["Transport"])
		require.NoError(t, err)

		err = conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
			Header: base.Header{
				"Transport": headers.Transport{
					Delivery: func() *headers.TransportDelivery {
						v := headers.TransportDeliveryUnicast
						return &v
					}(),
					Protocol:       headers.TransportProtocolTCP,
					InterleavedIDs: inTH.InterleavedIDs,
				}.Marshal(),
			},
		})
		require.NoError(t, err)

		req, err = conn.ReadRequest()
		require.NoError(t, err)
		require.Equal(t, base.Play, req.Method)

		err = conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
		})
		require.NoError(t, err)

		<-connected

		byts, _ := rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		}.Marshal()
		err = conn.WriteInterleavedFrame(&base.InterleavedFrame{
			Channel: 0,
			Payload: byts,
		}, make([]byte, 1024))
		require.NoError(t, err)

		byts, _ = rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 124,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 2000/4),
		}.Marshal()
		err = conn.WriteInterleavedFrame(&base.InterleavedFrame{
			Channel: 0,
			Payload: byts,
		}, make([]byte, 2048))
		require.NoError(t, err)

		byts, _ = rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 125,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		}.Marshal()
		err = conn.WriteInterleavedFrame(&base.InterleavedFrame{
			Channel: 0,
			Payload: byts,
		}, make([]byte, 1024))
		require.NoError(t, err)

		req, err = conn.ReadRequest()
		require.NoError(t, err)
		require.Equal(t, base.Teardown, req.Method)

		err = conn.WriteResponse(&base.Response{
			StatusCode: base.StatusOK,
		})
		require.NoError(t, err)
	}()

	p, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://127.0.0.1:8555/teststream\n" +
		"    sourceProtocol: tcp\n")
	require.Equal(t, true, ok)
	defer p.Close()

	time.Sleep(1 * time.Second)

	packetRecv := make(chan struct{})
	i := 0

	c := gortsplib.Client{
		OnPacketRTP: func(ctx *gortsplib.ClientOnPacketRTPCtx) {
			switch i {
			case 0:
				require.Equal(t, &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 123,
						Timestamp:      45343,
						SSRC:           563423,
						CSRC:           []uint32{},
					},
					Payload: []byte{0x01, 0x02, 0x03, 0x04},
				}, ctx.Packet)

			case 1:
				require.Equal(t, &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Marker:         false,
						PayloadType:    96,
						SequenceNumber: 124,
						Timestamp:      45343,
						SSRC:           563423,
						CSRC:           []uint32{},
					},
					Payload: append(
						append([]byte{0x1c, 0x81, 0x02, 0x03, 0x04}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 363)...),
						[]byte{0x01, 0x02, 0x03}...,
					),
				}, ctx.Packet)

			case 2:
				require.Equal(t, &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 125,
						Timestamp:      45343,
						SSRC:           563423,
						CSRC:           []uint32{},
					},
					Payload: append(
						[]byte{0x1c, 0x41, 0x04},
						bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 135)...,
					),
				}, ctx.Packet)

			case 3:
				require.Equal(t, &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Marker:         true,
						PayloadType:    96,
						SequenceNumber: 126,
						Timestamp:      45343,
						SSRC:           563423,
						CSRC:           []uint32{},
					},
					Payload: []byte{0x01, 0x02, 0x03, 0x04},
				}, ctx.Packet)
				close(packetRecv)
			}
			i++
		},
	}

	u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	tracks, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	err = c.SetupAndPlay(tracks, baseURL)
	require.NoError(t, err)

	close(connected)
	<-packetRecv
}
