package hls

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/codecs/h264"
	"github.com/aler9/gortsplib/v2/pkg/format"
	"github.com/asticode/go-astits"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/hls/fmp4"
	"github.com/aler9/rtsp-simple-server/internal/logger"
)

type testLogger struct{}

func (testLogger) Log(level logger.Level, format string, args ...interface{}) {
	log.Printf(format, args...)
}

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

func mpegtsSegment(w io.Writer) {
	mux := astits.NewMuxer(context.Background(), w)
	mux.AddElementaryStream(astits.PMTElementaryStream{
		ElementaryPID: 256,
		StreamType:    astits.StreamTypeH264Video,
	})
	mux.SetPCRPID(256)
	mux.WriteTables()

	enc, _ := h264.AnnexBMarshal([][]byte{
		{7, 1, 2, 3}, // SPS
		{8},          // PPS
		{5},          // IDR
	})

	mux.WriteData(&astits.MuxerData{
		PID: 256,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: astits.PTSDTSIndicatorBothPresent,
					PTS:             &astits.ClockReference{Base: 90000},                   // +1 sec
					DTS:             &astits.ClockReference{Base: 0x1FFFFFFFF - 90000 + 1}, // -1 sec
				},
				StreamID: 224, // = video
			},
			Data: enc,
		},
	})
}

func mp4Init(t *testing.T, w io.Writer) {
	i := &fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Format: &format.H264{
					SPS: []byte{
						0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
						0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
						0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9,
						0x20,
					},
					PPS: []byte{0x01, 0x02, 0x03, 0x04},
				},
			},
		},
	}

	byts, err := i.Marshal()
	require.NoError(t, err)

	_, err = w.Write(byts)
	require.NoError(t, err)
}

func mp4Segment(t *testing.T, w io.Writer) {
	payload, _ := h264.AVCCMarshal([][]byte{
		{7, 1, 2, 3}, // SPS
		{8},          // PPS
		{5},          // IDR
	})

	p := &fmp4.Part{
		Tracks: []*fmp4.PartTrack{
			{
				ID:      1,
				IsVideo: true,
				Samples: []*fmp4.PartSample{{
					Duration:  90000 / 30,
					PTSOffset: 90000 * 2,
					Payload:   payload,
				}},
			},
		},
	}

	byts, err := p.Marshal()
	require.NoError(t, err)

	_, err = w.Write(byts)
	require.NoError(t, err)
}

type testHLSServer struct {
	s *http.Server
}

func newTestHLSServer(router http.Handler, isTLS bool) (*testHLSServer, error) {
	ln, err := net.Listen("tcp", "localhost:5780")
	if err != nil {
		return nil, err
	}

	s := &testHLSServer{
		s: &http.Server{Handler: router},
	}

	if isTLS {
		go func() {
			serverCertFpath, err := writeTempFile(serverCert)
			if err != nil {
				panic(err)
			}
			defer os.Remove(serverCertFpath)

			serverKeyFpath, err := writeTempFile(serverKey)
			if err != nil {
				panic(err)
			}
			defer os.Remove(serverKeyFpath)

			s.s.ServeTLS(ln, serverCertFpath, serverKeyFpath)
		}()
	} else {
		go s.s.Serve(ln)
	}

	return s, nil
}

func (s *testHLSServer) close() {
	s.s.Shutdown(context.Background())
}

func TestClientMPEGTS(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"tls",
		"segment with query",
	} {
		t.Run(ca, func(t *testing.T) {
			gin.SetMode(gin.ReleaseMode)
			router := gin.New()

			segment := "segment.ts"
			if ca == "segment with query" {
				segment = "segment.ts?key=val"
			}
			sent := false

			router.GET("/stream.m3u8", func(ctx *gin.Context) {
				if sent {
					return
				}
				sent = true

				ctx.Writer.Header().Set("Content-Type", `application/x-mpegURL`)
				io.Copy(ctx.Writer, bytes.NewReader([]byte(`#EXTM3U
				#EXT-X-VERSION:3
				#EXT-X-ALLOW-CACHE:NO
				#EXT-X-TARGETDURATION:2
				#EXT-X-MEDIA-SEQUENCE:0
				#EXTINF:2,
				`+segment+`
				#EXT-X-ENDLIST
				`)))
			})

			router.GET("/segment.ts", func(ctx *gin.Context) {
				if ca == "segment with query" {
					require.Equal(t, "val", ctx.Query("key"))
				}
				ctx.Writer.Header().Set("Content-Type", `video/MP2T`)
				mpegtsSegment(ctx.Writer)
			})

			s, err := newTestHLSServer(router, ca == "tls")
			require.NoError(t, err)
			defer s.close()

			packetRecv := make(chan struct{})

			prefix := "http"
			if ca == "tls" {
				prefix = "https"
			}

			c, err := NewClient(
				prefix+"://localhost:5780/stream.m3u8",
				"33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739",
				func(videoTrack *format.H264, audioTrack *format.MPEG4Audio) error {
					require.Equal(t, &format.H264{
						PayloadTyp:        96,
						PacketizationMode: 1,
					}, videoTrack)
					require.Equal(t, (*format.MPEG4Audio)(nil), audioTrack)
					return nil
				},
				func(pts time.Duration, nalus [][]byte) {
					require.Equal(t, 2*time.Second, pts)
					require.Equal(t, [][]byte{
						{7, 1, 2, 3},
						{8},
						{5},
					}, nalus)
					close(packetRecv)
				},
				func(pts time.Duration, au []byte) {
				},
				testLogger{},
			)
			require.NoError(t, err)

			<-packetRecv

			c.Close()
			<-c.Wait()
		})
	}
}

func TestClientFMP4(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.GET("/stream.m3u8", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `application/x-mpegURL`)
		io.Copy(ctx.Writer, bytes.NewReader([]byte(`#EXTM3U
		#EXT-X-VERSION:7
		#EXT-X-MEDIA-SEQUENCE:20
		#EXT-X-INDEPENDENT-SEGMENTS
		#EXT-X-MAP:URI="init.mp4"
		#EXTINF:2,
		segment.mp4
		#EXT-X-ENDLIST
		`)))
	})

	router.GET("/init.mp4", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)
		mp4Init(t, ctx.Writer)
	})

	router.GET("/segment.mp4", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/mp4`)
		mp4Segment(t, ctx.Writer)
	})

	s, err := newTestHLSServer(router, false)
	require.NoError(t, err)
	defer s.close()

	packetRecv := make(chan struct{})

	c, err := NewClient(
		"http://localhost:5780/stream.m3u8",
		"",
		func(videoTrack *format.H264, audioTrack *format.MPEG4Audio) error {
			require.Equal(t, &format.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
				SPS:               videoTrack.SPS,
				PPS:               videoTrack.PPS,
			}, videoTrack)
			require.Equal(t, (*format.MPEG4Audio)(nil), audioTrack)
			return nil
		},
		func(pts time.Duration, nalus [][]byte) {
			require.Equal(t, 2*time.Second, pts)
			require.Equal(t, [][]byte{
				{7, 1, 2, 3},
				{8},
				{5},
			}, nalus)
			close(packetRecv)
		},
		func(pts time.Duration, au []byte) {
		},
		testLogger{},
	)
	require.NoError(t, err)

	<-packetRecv

	c.Close()
	<-c.Wait()
}

func TestClientInvalidSequenceID(t *testing.T) {
	router := gin.New()
	firstPlaylist := true

	router.GET("/stream.m3u8", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `application/x-mpegURL`)

		if firstPlaylist {
			firstPlaylist = false
			io.Copy(ctx.Writer, bytes.NewReader([]byte(
				`#EXTM3U
			#EXT-X-VERSION:3
			#EXT-X-ALLOW-CACHE:NO
			#EXT-X-TARGETDURATION:2
			#EXT-X-MEDIA-SEQUENCE:2
			#EXTINF:2,
			segment1.ts
			#EXTINF:2,
			segment1.ts
			#EXTINF:2,
			segment1.ts
			`)))
		} else {
			io.Copy(ctx.Writer, bytes.NewReader([]byte(
				`#EXTM3U
			#EXT-X-VERSION:3
			#EXT-X-ALLOW-CACHE:NO
			#EXT-X-TARGETDURATION:2
			#EXT-X-MEDIA-SEQUENCE:4
			#EXTINF:2,
			segment1.ts
			#EXTINF:2,
			segment1.ts
			#EXTINF:2,
			segment1.ts
			`)))
		}
	})

	router.GET("/segment1.ts", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", `video/MP2T`)
		mpegtsSegment(ctx.Writer)
	})

	s, err := newTestHLSServer(router, false)
	require.NoError(t, err)
	defer s.close()

	packetRecv := make(chan struct{})

	c, err := NewClient(
		"http://localhost:5780/stream.m3u8",
		"",
		func(*format.H264, *format.MPEG4Audio) error {
			return nil
		},
		func(pts time.Duration, nalus [][]byte) {
			close(packetRecv)
		},
		nil,
		testLogger{},
	)
	require.NoError(t, err)

	<-packetRecv

	// c.Close()
	err = <-c.Wait()
	require.EqualError(t, err, "following segment not found or not ready yet")

	c.Close()
}
