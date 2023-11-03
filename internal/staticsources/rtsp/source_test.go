package rtsp

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/staticsources/tester"
)

var serverCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUXw1hEC3LFpTsllv7D3ARJyEq7sIwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMDEyMTMxNzQ0NThaFw0zMDEy
MTExNzQ0NThaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDG8DyyS51810GsGwgWr5rjJK7OE1kTTLSNEEKax8Bj
zOyiaz8rA2JGl2VUEpi2UjDr9Cm7nd+YIEVs91IIBOb7LGqObBh1kGF3u5aZxLkv
NJE+HrLVvUhaDobK2NU+Wibqc/EI3DfUkt1rSINvv9flwTFu1qHeuLWhoySzDKEp
OzYxpFhwjVSokZIjT4Red3OtFz7gl2E6OAWe2qoh5CwLYVdMWtKR0Xuw3BkDPk9I
qkQKx3fqv97LPEzhyZYjDT5WvGrgZ1WDAN3booxXF3oA1H3GHQc4m/vcLatOtb8e
nI59gMQLEbnp08cl873bAuNuM95EZieXTHNbwUnq5iybAgMBAAGjUzBRMB0GA1Ud
DgQWBBQBKhJh8eWu0a4au9X/2fKhkFX2vjAfBgNVHSMEGDAWgBQBKhJh8eWu0a4a
u9X/2fKhkFX2vjAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBj
3aCW0YPKukYgVK9cwN0IbVy/D0C1UPT4nupJcy/E0iC7MXPZ9D/SZxYQoAkdptdO
xfI+RXkpQZLdODNx9uvV+cHyZHZyjtE5ENu/i5Rer2cWI/mSLZm5lUQyx+0KZ2Yu
tEI1bsebDK30msa8QSTn0WidW9XhFnl3gRi4wRdimcQapOWYVs7ih+nAlSvng7NI
XpAyRs8PIEbpDDBMWnldrX4TP6EWYUi49gCp8OUDRREKX3l6Ls1vZ02F34yHIt/7
7IV/XSKG096bhW+icKBWV0IpcEsgTzPK1J1hMxgjhzIMxGboAeUU+kidthOob6Sd
XQxaORfgM//NzX9LhUPk
-----END CERTIFICATE-----
`)

var serverKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAxvA8skudfNdBrBsIFq+a4ySuzhNZE0y0jRBCmsfAY8zsoms/
KwNiRpdlVBKYtlIw6/Qpu53fmCBFbPdSCATm+yxqjmwYdZBhd7uWmcS5LzSRPh6y
1b1IWg6GytjVPlom6nPxCNw31JLda0iDb7/X5cExbtah3ri1oaMkswyhKTs2MaRY
cI1UqJGSI0+EXndzrRc+4JdhOjgFntqqIeQsC2FXTFrSkdF7sNwZAz5PSKpECsd3
6r/eyzxM4cmWIw0+Vrxq4GdVgwDd26KMVxd6ANR9xh0HOJv73C2rTrW/HpyOfYDE
CxG56dPHJfO92wLjbjPeRGYnl0xzW8FJ6uYsmwIDAQABAoIBACi0BKcyQ3HElSJC
kaAao+Uvnzh4yvPg8Nwf5JDIp/uDdTMyIEWLtrLczRWrjGVZYbsVROinP5VfnPTT
kYwkfKINj2u+gC6lsNuPnRuvHXikF8eO/mYvCTur1zZvsQnF5kp4GGwIqr+qoPUP
bB0UMndG1PdpoMryHe+JcrvTrLHDmCeH10TqOwMsQMLHYLkowvxwJWsmTY7/Qr5S
Wm3PPpOcW2i0uyPVuyuv4yD1368fqnqJ8QFsQp1K6QtYsNnJ71Hut1/IoxK/e6hj
5Z+byKtHVtmcLnABuoOT7BhleJNFBksX9sh83jid4tMBgci+zXNeGmgqo2EmaWAb
agQslkECgYEA8B1rzjOHVQx/vwSzDa4XOrpoHQRfyElrGNz9JVBvnoC7AorezBXQ
M9WTHQIFTGMjzD8pb+YJGi3gj93VN51r0SmJRxBaBRh1ZZI9kFiFzngYev8POgD3
ygmlS3kTHCNxCK/CJkB+/jMBgtPj5ygDpCWVcTSuWlQFphePkW7jaaECgYEA1Blz
ulqgAyJHZaqgcbcCsI2q6m527hVr9pjzNjIVmkwu38yS9RTCgdlbEVVDnS0hoifl
+jVMEGXjF3xjyMvL50BKbQUH+KAa+V4n1WGlnZOxX9TMny8MBjEuSX2+362vQ3BX
4vOlX00gvoc+sY+lrzvfx/OdPCHQGVYzoKCxhLsCgYA07HcviuIAV/HsO2/vyvhp
xF5gTu+BqNUHNOZDDDid+ge+Jre2yfQLCL8VPLXIQW3Jff53IH/PGl+NtjphuLvj
7UDJvgvpZZuymIojP6+2c3gJ3CASC9aR3JBnUzdoE1O9s2eaoMqc4scpe+SWtZYf
3vzSZ+cqF6zrD/Rf/M35IQKBgHTU4E6ShPm09CcoaeC5sp2WK8OevZw/6IyZi78a
r5Oiy18zzO97U/k6xVMy6F+38ILl/2Rn31JZDVJujniY6eSkIVsUHmPxrWoXV1HO
y++U32uuSFiXDcSLarfIsE992MEJLSAynbF1Rsgsr3gXbGiuToJRyxbIeVy7gwzD
94TpAoGAY4/PejWQj9psZfAhyk5dRGra++gYRQ/gK1IIc1g+Dd2/BxbT/RHr05GK
6vwrfjsoRyMWteC1SsNs/CurjfQ/jqCfHNP5XPvxgd5Ec8sRJIiV7V5RTuWJsPu1
+3K6cnKEyg+0ekYmLertRFIY6SwWmY1fyKgTvxudMcsBY7dC4xs=
-----END RSA PRIVATE KEY-----
`)

func writeTempFile(byts []byte) (string, error) {
	tmpf, err := os.CreateTemp(os.TempDir(), "rtsp-")
	if err != nil {
		return "", err
	}
	defer tmpf.Close()

	_, err = tmpf.Write(byts)
	if err != nil {
		return "", err
	}

	return tmpf.Name(), nil
}

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

var testMediaH264 = &description.Media{
	Type: description.MediaTypeVideo,
	Formats: []format.Format{&format.H264{
		PayloadTyp: 96,
		SPS: []byte{ // 1920x1080 baseline
			0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
			0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
			0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
		},
		PPS:               []byte{0x08, 0x06, 0x07, 0x08},
		PacketizationMode: 1,
	}},
}

func TestRTSPSource(t *testing.T) {
	for _, source := range []string{
		"udp",
		"tcp",
		"tls",
	} {
		t.Run(source, func(t *testing.T) {
			var stream *gortsplib.ServerStream

			nonce, err := auth.GenerateNonce()
			require.NoError(t, err)

			s := gortsplib.Server{
				Handler: &testServer{
					onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx,
					) (*base.Response, *gortsplib.ServerStream, error) {
						err := auth.Validate(ctx.Request, "testuser", "testpass", nil, nil, "IPCAM", nonce)
						if err != nil {
							return &base.Response{ //nolint:nilerr
								StatusCode: base.StatusUnauthorized,
								Header: base.Header{
									"WWW-Authenticate": auth.GenerateWWWAuthenticate(nil, "IPCAM", nonce),
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
							time.Sleep(100 * time.Millisecond)
							err := stream.WritePacketRTP(testMediaH264, &rtp.Packet{
								Header: rtp.Header{
									Version:        0x02,
									PayloadType:    96,
									SequenceNumber: 57899,
									Timestamp:      345234345,
									SSRC:           978651231,
									Marker:         true,
								},
								Payload: []byte{5, 1, 2, 3, 4},
							})
							require.NoError(t, err)
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

			err = s.Start()
			require.NoError(t, err)
			defer s.Wait() //nolint:errcheck
			defer s.Close()

			stream = gortsplib.NewServerStream(&s, &description.Session{Medias: []*description.Media{testMediaH264}})
			defer stream.Close()

			var te *tester.Tester

			if source != "tls" {
				var sp conf.RTSPTransport
				sp.UnmarshalJSON([]byte(`"` + source + `"`)) //nolint:errcheck

				te = tester.New(
					func(p defs.StaticSourceParent) defs.StaticSource {
						return &Source{
							ReadTimeout:    conf.StringDuration(10 * time.Second),
							WriteTimeout:   conf.StringDuration(10 * time.Second),
							WriteQueueSize: 2048,
							Parent:         p,
						}
					},
					&conf.Path{
						Source:        "rtsp://testuser:testpass@localhost:8555/teststream",
						RTSPTransport: sp,
					},
				)
			} else {
				te = tester.New(
					func(p defs.StaticSourceParent) defs.StaticSource {
						return &Source{
							ReadTimeout:    conf.StringDuration(10 * time.Second),
							WriteTimeout:   conf.StringDuration(10 * time.Second),
							WriteQueueSize: 2048,
							Parent:         p,
						}
					},
					&conf.Path{
						Source:            "rtsps://testuser:testpass@localhost:8555/teststream",
						SourceFingerprint: "33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739",
					},
				)
			}

			defer te.Close()

			<-te.Unit
		})
	}
}

func TestRTSPSourceNoPassword(t *testing.T) {
	var stream *gortsplib.ServerStream

	nonce, err := auth.GenerateNonce()
	require.NoError(t, err)

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				err := auth.Validate(ctx.Request, "testuser", "", nil, nil, "IPCAM", nonce)
				if err != nil {
					return &base.Response{ //nolint:nilerr
						StatusCode: base.StatusUnauthorized,
						Header: base.Header{
							"WWW-Authenticate": auth.GenerateWWWAuthenticate(nil, "IPCAM", nonce),
						},
					}, nil, nil
				}

				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onSetup: func(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				go func() {
					time.Sleep(100 * time.Millisecond)
					err := stream.WritePacketRTP(testMediaH264, &rtp.Packet{
						Header: rtp.Header{
							Version:        0x02,
							PayloadType:    96,
							SequenceNumber: 57899,
							Timestamp:      345234345,
							SSRC:           978651231,
							Marker:         true,
						},
						Payload: []byte{5, 1, 2, 3, 4},
					})
					require.NoError(t, err)
				}()

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

	err = s.Start()
	require.NoError(t, err)
	defer s.Wait() //nolint:errcheck
	defer s.Close()

	stream = gortsplib.NewServerStream(&s, &description.Session{Medias: []*description.Media{testMediaH264}})
	defer stream.Close()

	var sp conf.RTSPTransport
	sp.UnmarshalJSON([]byte(`"tcp"`)) //nolint:errcheck

	te := tester.New(
		func(p defs.StaticSourceParent) defs.StaticSource {
			return &Source{
				ReadTimeout:    conf.StringDuration(10 * time.Second),
				WriteTimeout:   conf.StringDuration(10 * time.Second),
				WriteQueueSize: 2048,
				Parent:         p,
			}
		},
		&conf.Path{
			Source:        "rtsp://testuser:@127.0.0.1:8555/teststream",
			RTSPTransport: sp,
		},
	)
	defer te.Close()

	<-te.Unit
}

func TestRTSPSourceRange(t *testing.T) {
	for _, ca := range []string{"clock", "npt", "smpte"} {
		t.Run(ca, func(t *testing.T) {
			var stream *gortsplib.ServerStream

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
						switch ca {
						case "clock":
							require.Equal(t, base.HeaderValue{"clock=20230812T120000Z-"}, ctx.Request.Header["Range"])

						case "npt":
							require.Equal(t, base.HeaderValue{"npt=0.35-"}, ctx.Request.Header["Range"])

						case "smpte":
							require.Equal(t, base.HeaderValue{"smpte=0:02:10-"}, ctx.Request.Header["Range"])
						}

						go func() {
							time.Sleep(100 * time.Millisecond)
							err := stream.WritePacketRTP(testMediaH264, &rtp.Packet{
								Header: rtp.Header{
									Version:        0x02,
									PayloadType:    96,
									SequenceNumber: 57899,
									Timestamp:      345234345,
									SSRC:           978651231,
									Marker:         true,
								},
								Payload: []byte{5, 1, 2, 3, 4},
							})
							require.NoError(t, err)
						}()

						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
				},
				RTSPAddress: "127.0.0.1:8555",
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Wait() //nolint:errcheck
			defer s.Close()

			stream = gortsplib.NewServerStream(&s, &description.Session{Medias: []*description.Media{testMediaH264}})
			defer stream.Close()

			cnf := &conf.Path{
				Source: "rtsp://127.0.0.1:8555/teststream",
			}

			switch ca {
			case "clock":
				cnf.RTSPRangeType = conf.RTSPRangeTypeClock
				cnf.RTSPRangeStart = "20230812T120000Z"

			case "npt":
				cnf.RTSPRangeType = conf.RTSPRangeTypeNPT
				cnf.RTSPRangeStart = "350ms"

			case "smpte":
				cnf.RTSPRangeType = conf.RTSPRangeTypeSMPTE
				cnf.RTSPRangeStart = "130s"
			}

			te := tester.New(
				func(p defs.StaticSourceParent) defs.StaticSource {
					return &Source{
						ReadTimeout:    conf.StringDuration(10 * time.Second),
						WriteTimeout:   conf.StringDuration(10 * time.Second),
						WriteQueueSize: 2048,
						Parent:         p,
					}
				},
				cnf,
			)
			defer te.Close()

			<-te.Unit
		})
	}
}
