package core

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/auth"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/stretchr/testify/require"
)

type testServer struct {
	user          string
	pass          string
	authValidator *auth.Validator
	stream        *gortsplib.ServerStream

	done chan struct{}
}

func (sh *testServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	if sh.authValidator == nil {
		sh.authValidator = auth.NewValidator(sh.user, sh.pass, nil)
	}

	err := sh.authValidator.ValidateRequest(ctx.Req, nil)
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusUnauthorized,
			Header: base.Header{
				"WWW-Authenticate": sh.authValidator.Header(),
			},
		}, nil, nil
	}

	track, _ := gortsplib.NewTrackH264(96,
		&gortsplib.TrackConfigH264{SPS: []byte{0x01, 0x02, 0x03, 0x04}, PPS: []byte{0x05, 0x06}})
	sh.stream = gortsplib.NewServerStream(gortsplib.Tracks{track})

	return &base.Response{
		StatusCode: base.StatusOK,
	}, sh.stream, nil
}

func (sh *testServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if sh.done != nil {
		close(sh.done)
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, sh.stream, nil
}

func (sh *testServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	go func() {
		time.Sleep(1 * time.Second)
		sh.stream.WriteFrame(0, gortsplib.StreamTypeRTP, []byte{0x01, 0x02, 0x03, 0x04})
	}()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func TestRTSPSource(t *testing.T) {
	for _, source := range []string{
		"udp",
		"tcp",
		"tls",
	} {
		t.Run(source, func(t *testing.T) {
			s := gortsplib.Server{
				Handler: &testServer{user: "testuser", pass: "testpass"},
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

			err := s.Start("127.0.0.1:8555")
			require.NoError(t, err)
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

			conn, err := gortsplib.DialRead("rtsp://127.0.0.1:8554/proxied")
			require.NoError(t, err)

			readDone := make(chan struct{})
			received := make(chan struct{})
			go func() {
				defer close(readDone)
				conn.ReadFrames(func(trackID int, streamType gortsplib.StreamType, payload []byte) {
					if streamType == gortsplib.StreamTypeRTP {
						require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, payload)
						close(received)
					}
				})
			}()

			<-received
			conn.Close()
			<-readDone
		})
	}
}

func TestRTSPSourceNoPassword(t *testing.T) {
	done := make(chan struct{})
	s := gortsplib.Server{Handler: &testServer{user: "testuser", done: done}}
	err := s.Start("127.0.0.1:8555")
	require.NoError(t, err)
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
