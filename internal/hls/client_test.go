package hls

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/h264"
	"github.com/asticode/go-astits"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

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
	tmpf, err := ioutil.TempFile(os.TempDir(), "rtsp-")
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

type testHLSServer struct {
	s *http.Server
}

func newTestHLSServer(ca string) (*testHLSServer, error) {
	ln, err := net.Listen("tcp", "localhost:5780")
	if err != nil {
		return nil, err
	}

	ts := &testHLSServer{}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	segment := "segment.ts"
	if ca == "segment with query" {
		segment = "segment.ts?key=val"
	}

	router.GET("/stream.m3u8", func(ctx *gin.Context) {
		cnt := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-ALLOW-CACHE:NO
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:2,
` + segment + "\n"

		ctx.Writer.Header().Set("Content-Type", `audio/mpegURL`)
		io.Copy(ctx.Writer, bytes.NewReader([]byte(cnt)))
	})

	router.GET("/segment.ts", func(ctx *gin.Context) {
		if ca == "segment with query" && ctx.Query("key") != "val" {
			return
		}

		ctx.Writer.Header().Set("Content-Type", `video/MP2T`)

		mux := astits.NewMuxer(context.Background(), ctx.Writer)
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
						PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS,
						PTS:             &astits.ClockReference{Base: int64(1 * 90000)},
					},
					StreamID: 224, // = video
				},
				Data: enc,
			},
		})
	})

	ts.s = &http.Server{Handler: router}

	if ca == "tls" {
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

			ts.s.ServeTLS(ln, serverCertFpath, serverKeyFpath)
		}()
	} else {
		go ts.s.Serve(ln)
	}

	return ts, nil
}

func (ts *testHLSServer) close() {
	ts.s.Shutdown(context.Background())
}

func TestClient(t *testing.T) {
	for _, ca := range []string{
		"plain",
		"tls",
		"segment with query",
	} {
		t.Run(ca, func(t *testing.T) {
			ts, err := newTestHLSServer(ca)
			require.NoError(t, err)
			defer ts.close()

			packetRecv := make(chan struct{})

			prefix := "http"
			if ca == "tls" {
				prefix = "https"
			}

			c, err := NewClient(
				prefix+"://localhost:5780/stream.m3u8",
				"33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739",
				func(*gortsplib.TrackH264, *gortsplib.TrackAAC) error {
					return nil
				},
				func(pts time.Duration, nalus [][]byte) {
					require.Equal(t, [][]byte{
						{7, 1, 2, 3},
						{8},
						{0x05},
					}, nalus)
					close(packetRecv)
				},
				func(pts time.Duration, aus [][]byte) {
				},
				testLogger{},
			)
			require.NoError(t, err)

			<-packetRecv

			c.Close()
			c.Wait()
		})
	}
}
