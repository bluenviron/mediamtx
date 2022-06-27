package core

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/headers"
	"github.com/aler9/gortsplib/pkg/url"
	"github.com/stretchr/testify/require"
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

type container struct {
	name string
}

func newContainer(image string, name string, args []string) (*container, error) {
	c := &container{
		name: name,
	}

	exec.Command("docker", "kill", "rtsp-simple-server-test-"+name).Run()
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+name).Run()

	// --network=host is needed to test multicast
	cmd := []string{
		"docker", "run",
		"--network=host",
		"--name=rtsp-simple-server-test-" + name,
		"rtsp-simple-server-test-" + image,
	}
	cmd = append(cmd, args...)
	ecmd := exec.Command(cmd[0], cmd[1:]...)
	ecmd.Stdout = nil
	ecmd.Stderr = os.Stderr

	err := ecmd.Start()
	if err != nil {
		return nil, err
	}

	time.Sleep(1 * time.Second)

	return c, nil
}

func (c *container) close() {
	exec.Command("docker", "kill", "rtsp-simple-server-test-"+c.name).Run()
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+c.name).Run()
	exec.Command("docker", "rm", "rtsp-simple-server-test-"+c.name).Run()
}

func (c *container) wait() int {
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+c.name).Run()
	out, _ := exec.Command("docker", "inspect", "rtsp-simple-server-test-"+c.name,
		"-f", "{{.State.ExitCode}}").Output()
	code, _ := strconv.ParseInt(string(out[:len(out)-1]), 10, 64)
	return int(code)
}

func (c *container) ip() string {
	out, _ := exec.Command("docker", "inspect", "rtsp-simple-server-test-"+c.name,
		"-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}").Output()
	return string(out[:len(out)-1])
}

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

func newInstance(conf string) (*Core, bool) {
	if conf == "" {
		return New([]string{})
	}

	tmpf, err := writeTempFile([]byte(conf))
	if err != nil {
		return nil, false
	}
	defer os.Remove(tmpf)

	return New([]string{tmpf})
}

func TestCorePathAutoDeletion(t *testing.T) {
	for _, ca := range []string{"describe", "setup"} {
		t.Run(ca, func(t *testing.T) {
			p, ok := newInstance("paths:\n" +
				"  all:\n")
			require.Equal(t, true, ok)
			defer p.close()

			func() {
				conn, err := net.Dial("tcp", "localhost:8554")
				require.NoError(t, err)
				defer conn.Close()
				br := bufio.NewReader(conn)

				if ca == "describe" {
					u, err := url.Parse("rtsp://localhost:8554/mypath")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Describe,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Read(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusNotFound, res.StatusCode)
				} else {
					u, err := url.Parse("rtsp://localhost:8554/mypath/trackID=0")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Setup,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
							"Transport": headers.Transport{
								Mode: func() *headers.TransportMode {
									v := headers.TransportModePlay
									return &v
								}(),
								Delivery: func() *headers.TransportDelivery {
									v := headers.TransportDeliveryUnicast
									return &v
								}(),
								Protocol:    headers.TransportProtocolUDP,
								ClientPorts: &[2]int{35466, 35467},
							}.Marshal(),
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Read(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusNotFound, res.StatusCode)
				}
			}()

			res := p.pathManager.onAPIPathsList(pathAPIPathsListReq{})
			require.NoError(t, res.err)

			require.Equal(t, 0, len(res.data.Items))
		})
	}
}

func TestCorePathRunOnDemand(t *testing.T) {
	doneFile := filepath.Join(os.TempDir(), "ondemand_done")

	srcFile := filepath.Join(os.TempDir(), "ondemand.go")
	err := ioutil.WriteFile(srcFile, []byte(`
package main

import (
	"os"
	"os/signal"
	"syscall"
	"io/ioutil"
	"github.com/aler9/gortsplib"
)

func main() {
	if os.Getenv("G1") != "on" {
		panic("environment not set")
	}

	track := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS: []byte{0x01, 0x02, 0x03, 0x04},
		PPS: []byte{0x01, 0x02, 0x03, 0x04},
	}

	source := gortsplib.Client{}

	err := source.StartPublishing(
		"rtsp://localhost:" + os.Getenv("RTSP_PORT") + "/" + os.Getenv("RTSP_PATH"),
		gortsplib.Tracks{track})
	if err != nil {
		panic(err)
	}
	defer source.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	<-c

	err = ioutil.WriteFile("`+doneFile+`", []byte(""), 0644)
	if err != nil {
		panic(err)
	}
}
`), 0o644)
	require.NoError(t, err)

	execFile := filepath.Join(os.TempDir(), "ondemand_cmd")
	cmd := exec.Command("go", "build", "-o", execFile, srcFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	require.NoError(t, err)
	defer os.Remove(execFile)

	os.Remove(srcFile)

	for _, ca := range []string{"describe", "setup", "describe and setup"} {
		t.Run(ca, func(t *testing.T) {
			defer os.Remove(doneFile)

			p1, ok := newInstance(fmt.Sprintf("rtmpDisable: yes\n"+
				"hlsDisable: yes\n"+
				"paths:\n"+
				"  '~^(on)demand$':\n"+
				"    runOnDemand: %s\n"+
				"    runOnDemandCloseAfter: 1s\n", execFile))
			require.Equal(t, true, ok)
			defer p1.close()

			func() {
				conn, err := net.Dial("tcp", "localhost:8554")
				require.NoError(t, err)
				defer conn.Close()
				br := bufio.NewReader(conn)

				if ca == "describe" || ca == "describe and setup" {
					u, err := url.Parse("rtsp://localhost:8554/ondemand")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Describe,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"1"},
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Read(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusOK, res.StatusCode)
				}

				if ca == "setup" || ca == "describe and setup" {
					u, err := url.Parse("rtsp://localhost:8554/ondemand/trackID=0")
					require.NoError(t, err)

					byts, _ := base.Request{
						Method: base.Setup,
						URL:    u,
						Header: base.Header{
							"CSeq": base.HeaderValue{"2"},
							"Transport": headers.Transport{
								Mode: func() *headers.TransportMode {
									v := headers.TransportModePlay
									return &v
								}(),
								Protocol:       headers.TransportProtocolTCP,
								InterleavedIDs: &[2]int{0, 1},
							}.Marshal(),
						},
					}.Marshal()
					_, err = conn.Write(byts)
					require.NoError(t, err)

					var res base.Response
					err = res.Read(br)
					require.NoError(t, err)
					require.Equal(t, base.StatusOK, res.StatusCode)
				}
			}()

			for {
				_, err := os.Stat(doneFile)
				if err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		})
	}
}

func TestCorePathRunOnReady(t *testing.T) {
	doneFile := filepath.Join(os.TempDir(), "onready_done")
	defer os.Remove(doneFile)

	p, ok := newInstance(fmt.Sprintf("rtmpDisable: yes\n"+
		"hlsDisable: yes\n"+
		"paths:\n"+
		"  test:\n"+
		"    runOnReady: touch %s\n",
		doneFile))
	require.Equal(t, true, ok)
	defer p.close()

	track := &gortsplib.TrackH264{
		PayloadType: 96,
		SPS:         []byte{0x01, 0x02, 0x03, 0x04},
		PPS:         []byte{0x01, 0x02, 0x03, 0x04},
	}

	c := gortsplib.Client{}

	err := c.StartPublishing(
		"rtsp://localhost:8554/test",
		gortsplib.Tracks{track})
	require.NoError(t, err)
	defer c.Close()

	time.Sleep(1 * time.Second)

	_, err = os.Stat(doneFile)
	require.NoError(t, err)
}

func TestCoreHotReloading(t *testing.T) {
	confPath := filepath.Join(os.TempDir(), "rtsp-conf")

	err := ioutil.WriteFile(confPath, []byte("paths:\n"+
		"  test1:\n"+
		"    publishUser: myuser\n"+
		"    publishPass: mypass\n"),
		0o644)
	require.NoError(t, err)
	defer os.Remove(confPath)

	p, ok := New([]string{confPath})
	require.Equal(t, true, ok)
	defer p.close()

	func() {
		track := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS:         []byte{0x01, 0x02, 0x03, 0x04},
			PPS:         []byte{0x01, 0x02, 0x03, 0x04},
		}

		c := gortsplib.Client{}

		err = c.StartPublishing(
			"rtsp://localhost:8554/test1",
			gortsplib.Tracks{track})
		require.EqualError(t, err, "bad status code: 401 (Unauthorized)")
	}()

	err = ioutil.WriteFile(confPath, []byte("paths:\n"+
		"  test1:\n"),
		0o644)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	func() {
		track := &gortsplib.TrackH264{
			PayloadType: 96,
			SPS:         []byte{0x01, 0x02, 0x03, 0x04},
			PPS:         []byte{0x01, 0x02, 0x03, 0x04},
		}

		conn := gortsplib.Client{}

		err = conn.StartPublishing(
			"rtsp://localhost:8554/test1",
			gortsplib.Tracks{track})
		require.NoError(t, err)
		defer conn.Close()
	}()
}
