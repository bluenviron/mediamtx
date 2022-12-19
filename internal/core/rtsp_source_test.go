package core

import (
	"bytes"
	"crypto/tls"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/auth"
	"github.com/aler9/gortsplib/v2/pkg/base"
	"github.com/aler9/gortsplib/v2/pkg/conn"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/aler9/gortsplib/v2/pkg/h265"
	"github.com/aler9/gortsplib/v2/pkg/headers"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/url"
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
			medi := testMediaH264
			stream := gortsplib.NewServerStream(media.Medias{medi})

			var authValidator *auth.Validator

			s := gortsplib.Server{
				Handler: &testServer{
					onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx,
					) (*base.Response, *gortsplib.ServerStream, error) {
						if authValidator == nil {
							authValidator = auth.NewValidator("testuser", "testpass", nil)
						}

						err := authValidator.ValidateRequest(ctx.Request, nil)
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
							stream.WritePacketRTP(medi, &rtp.Packet{
								Header: rtp.Header{
									Version:        0x02,
									PayloadType:    96,
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

			c := gortsplib.Client{}

			u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			medias, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(medias, baseURL)
			require.NoError(t, err)

			c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
				require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, pkt.Payload)
				close(received)
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			<-received
		})
	}
}

func TestRTSPSourceNoPassword(t *testing.T) {
	medi := testMediaH264
	stream := gortsplib.NewServerStream(media.Medias{medi})

	var authValidator *auth.Validator
	done := make(chan struct{})

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				if authValidator == nil {
					authValidator = auth.NewValidator("testuser", "", nil)
				}

				err := authValidator.ValidateRequest(ctx.Request, nil)
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
		"webrtcDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://testuser:@127.0.0.1:8555/teststream\n" +
		"    sourceProtocol: tcp\n")
	require.Equal(t, true, ok)
	defer p.Close()

	<-done
}

func TestRTSPSourceDynamicH264Params(t *testing.T) {
	checkTrack := func(t *testing.T, forma format.Format) {
		c := gortsplib.Client{}

		u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
		require.NoError(t, err)

		err = c.Start(u.Scheme, u.Host)
		require.NoError(t, err)
		defer c.Close()

		medias, _, _, err := c.Describe(u)
		require.NoError(t, err)

		forma1 := medias[0].Formats[0]
		require.Equal(t, forma, forma1)
	}

	t.Run("h264", func(t *testing.T) {
		forma := &format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}
		medi := &media.Media{
			Type:    media.TypeVideo,
			Formats: []format.Format{forma},
		}
		stream := gortsplib.NewServerStream(media.Medias{medi})
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
			"webrtcDisable: yes\n" +
			"paths:\n" +
			"  proxied:\n" +
			"    source: rtsp://127.0.0.1:8555/teststream\n")
		require.Equal(t, true, ok)
		defer p.Close()

		time.Sleep(1 * time.Second)

		enc := forma.CreateEncoder()

		pkts, err := enc.Encode([][]byte{{7, 1, 2, 3}}, 0) // SPS
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		pkts, err = enc.Encode([][]byte{{8}}, 0) // PPS
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		checkTrack(t, &format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
			SPS:               []byte{7, 1, 2, 3},
			PPS:               []byte{8},
		})

		pkts, err = enc.Encode([][]byte{{7, 4, 5, 6}}, 0) // SPS
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		pkts, err = enc.Encode([][]byte{{8, 1}}, 0) // PPS
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		checkTrack(t, &format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
			SPS:               []byte{7, 4, 5, 6},
			PPS:               []byte{8, 1},
		})
	})

	t.Run("h265", func(t *testing.T) {
		forma := &format.H265{
			PayloadTyp: 96,
		}
		medi := &media.Media{
			Type:    media.TypeVideo,
			Formats: []format.Format{forma},
		}
		stream := gortsplib.NewServerStream(media.Medias{medi})
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
			"webrtcDisable: yes\n" +
			"paths:\n" +
			"  proxied:\n" +
			"    source: rtsp://127.0.0.1:8555/teststream\n")
		require.Equal(t, true, ok)
		defer p.Close()

		time.Sleep(1 * time.Second)

		enc := forma.CreateEncoder()

		pkts, err := enc.Encode([][]byte{{byte(h265.NALUTypeVPS) << 1, 1, 2, 3}}, 0)
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		pkts, err = enc.Encode([][]byte{{byte(h265.NALUTypeSPS) << 1, 4, 5, 6}}, 0)
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		pkts, err = enc.Encode([][]byte{{byte(h265.NALUTypePPS) << 1, 7, 8, 9}}, 0)
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		checkTrack(t, &format.H265{
			PayloadTyp: 96,
			VPS:        []byte{byte(h265.NALUTypeVPS) << 1, 1, 2, 3},
			SPS:        []byte{byte(h265.NALUTypeSPS) << 1, 4, 5, 6},
			PPS:        []byte{byte(h265.NALUTypePPS) << 1, 7, 8, 9},
		})

		pkts, err = enc.Encode([][]byte{{byte(h265.NALUTypeVPS) << 1, 10, 11, 12}}, 0)
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		pkts, err = enc.Encode([][]byte{{byte(h265.NALUTypeSPS) << 1, 13, 14, 15}}, 0)
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		pkts, err = enc.Encode([][]byte{{byte(h265.NALUTypePPS) << 1, 16, 17, 18}}, 0)
		require.NoError(t, err)
		stream.WritePacketRTP(medi, pkts[0])

		checkTrack(t, &format.H265{
			PayloadTyp: 96,
			VPS:        []byte{byte(h265.NALUTypeVPS) << 1, 10, 11, 12},
			SPS:        []byte{byte(h265.NALUTypeSPS) << 1, 13, 14, 15},
			PPS:        []byte{byte(h265.NALUTypePPS) << 1, 16, 17, 18},
		})
	})
}

func TestRTSPSourceRemovePadding(t *testing.T) {
	medi := testMediaH264
	stream := gortsplib.NewServerStream(media.Medias{medi})
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
		"webrtcDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://127.0.0.1:8555/teststream\n")
	require.Equal(t, true, ok)
	defer p.Close()

	time.Sleep(1 * time.Second)

	packetRecv := make(chan struct{})

	c := gortsplib.Client{}

	u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
	require.NoError(t, err)

	err = c.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer c.Close()

	medias, baseURL, _, err := c.Describe(u)
	require.NoError(t, err)

	err = c.SetupAll(medias, baseURL)
	require.NoError(t, err)

	c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
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
		}, pkt)
		close(packetRecv)
	})

	_, err = c.Play(nil)
	require.NoError(t, err)

	stream.WritePacketRTP(medi, &rtp.Packet{
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
	for _, ca := range []string{"h264", "h265"} {
		t.Run(ca, func(t *testing.T) {
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

				medias := media.Medias{testMediaH264}
				byts, _ := medias.Marshal(false).Marshal()

				err = conn.WriteResponse(&base.Response{
					StatusCode: base.StatusOK,
					Header: base.Header{
						"Content-Type": base.HeaderValue{"application/sdp"},
					},
					Body: byts,
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

				var tosend []*rtp.Packet
				if ca == "h264" {
					tosend = []*rtp.Packet{
						{
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
						},
						{
							Header: rtp.Header{
								Version:        2,
								Marker:         false,
								PayloadType:    96,
								SequenceNumber: 124,
								Timestamp:      45343,
								SSRC:           563423,
								Padding:        true,
							},
							Payload: append([]byte{0x1c, 0b10000000}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 2000/4)...),
						},
						{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: 125,
								Timestamp:      45343,
								SSRC:           563423,
								Padding:        true,
							},
							Payload: []byte{0x1c, 0b01000000, 0x01, 0x02, 0x03, 0x04},
						},
					}
				} else {
					tosend = []*rtp.Packet{
						{
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
						},
						{
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
						},
					}
				}

				for _, pkt := range tosend {
					byts, _ = pkt.Marshal()
					err = conn.WriteInterleavedFrame(&base.InterleavedFrame{
						Channel: 0,
						Payload: byts,
					}, make([]byte, 2048))
					require.NoError(t, err)
				}

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
				"webrtcDisable: yes\n" +
				"paths:\n" +
				"  proxied:\n" +
				"    source: rtsp://127.0.0.1:8555/teststream\n" +
				"    sourceProtocol: tcp\n")
			require.Equal(t, true, ok)
			defer p.Close()

			time.Sleep(1 * time.Second)

			c := gortsplib.Client{}

			u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			medias, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(medias, baseURL)
			require.NoError(t, err)

			packetRecv := make(chan struct{})
			i := 0

			var expected []*rtp.Packet

			if ca == "h264" {
				expected = []*rtp.Packet{
					{
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
					},
					{
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
							append([]byte{0x1c, 0x80}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 364)...),
							[]byte{0x01, 0x02}...,
						),
					},
					{
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
							[]byte{0x1c, 0x40, 0x03, 0x04},
							bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 136)...,
						),
					},
				}
			} else {
				expected = []*rtp.Packet{
					{
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
					},
					{
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
					},
					{
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
					},
				}
			}

			c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
				require.Equal(t, expected[i], pkt)
				i++
				if i >= len(expected) {
					close(packetRecv)
				}
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			close(connected)
			<-packetRecv
		})
	}
}
