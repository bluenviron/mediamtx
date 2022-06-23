package core

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/auth"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/rtph264"
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
				PayloadType: 96,
				SPS:         []byte{0x01, 0x02, 0x03, 0x04},
				PPS:         []byte{0x01, 0x02, 0x03, 0x04},
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
							}, true)
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
				defer p.close()
			} else {
				p, ok := newInstance("paths:\n" +
					"  proxied:\n" +
					"    source: rtsps://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceFingerprint: 33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p.close()
			}

			time.Sleep(1 * time.Second)

			received := make(chan struct{})

			c := gortsplib.Client{
				OnPacketRTP: func(ctx *gortsplib.ClientOnPacketRTPCtx) {
					require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, ctx.Packet.Payload)
					close(received)
				},
			}

			err = c.StartReading("rtsp://127.0.0.1:8554/proxied")
			require.NoError(t, err)
			defer c.Close()

			<-received
		})
	}
}

func TestRTSPSourceNoPassword(t *testing.T) {
	track := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
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
	defer p.close()

	<-done
}

func TestRTSPSourceDynamicH264Params(t *testing.T) {
	track := &gortsplib.TrackH264{
		PayloadType: 96,
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
	defer p.close()

	time.Sleep(1 * time.Second)

	enc := &rtph264.Encoder{PayloadType: 96}
	enc.Init()

	pkts, err := enc.Encode([][]byte{{7, 1, 2, 3}}, 0) // SPS
	require.NoError(t, err)
	stream.WritePacketRTP(0, pkts[0], true)

	pkts, err = enc.Encode([][]byte{{8}}, 0) // PPS
	require.NoError(t, err)
	stream.WritePacketRTP(0, pkts[0], true)

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
	stream.WritePacketRTP(0, pkts[0], true)

	pkts, err = enc.Encode([][]byte{{8, 1}}, 0) // PPS
	require.NoError(t, err)
	stream.WritePacketRTP(0, pkts[0], true)

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
