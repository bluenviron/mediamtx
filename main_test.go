package main

import (
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/stretchr/testify/require"

	"github.com/aler9/rtsp-simple-server/internal/conf"
)

var ownDockerIP = func() string {
	out, err := exec.Command("docker", "network", "inspect", "bridge",
		"-f", "{{range .IPAM.Config}}{{.Subnet}}{{end}}").Output()
	if err != nil {
		panic(err)
	}

	_, ipnet, err := net.ParseCIDR(string(out[:len(out)-1]))
	if err != nil {
		panic(err)
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if v, ok := addr.(*net.IPNet); ok {
				if ipnet.Contains(v.IP) {
					return v.IP.String()
				}
			}
		}
	}

	panic("IP not found")
}()

type container struct {
	name string
}

func newContainer(image string, name string, args []string) (*container, error) {
	c := &container{
		name: name,
	}

	exec.Command("docker", "kill", "rtsp-simple-server-test-"+name).Run()
	exec.Command("docker", "wait", "rtsp-simple-server-test-"+name).Run()

	cmd := []string{"docker", "run",
		"--name=rtsp-simple-server-test-" + name,
		"rtsp-simple-server-test-" + image}
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

func testProgram(conf string) (*program, bool) {
	if conf == "" {
		return newProgram([]string{})
	}

	tmpf, err := writeTempFile([]byte(conf))
	if err != nil {
		return nil, false
	}
	defer os.Remove(tmpf)

	return newProgram([]string{tmpf})
}

func TestEnvironment(t *testing.T) {
	// string
	os.Setenv("RTSP_RUNONCONNECT", "test=cmd")
	defer os.Unsetenv("RTSP_RUNONCONNECT")

	// int
	os.Setenv("RTSP_RTSPPORT", "8555")
	defer os.Unsetenv("RTSP_RTSPPORT")

	// bool
	os.Setenv("RTSP_METRICS", "yes")
	defer os.Unsetenv("RTSP_METRICS")

	// duration
	os.Setenv("RTSP_READTIMEOUT", "22s")
	defer os.Unsetenv("RTSP_READTIMEOUT")

	// slice
	os.Setenv("RTSP_LOGDESTINATIONS", "stdout,file")
	defer os.Unsetenv("RTSP_LOGDESTINATIONS")

	// map key
	os.Setenv("RTSP_PATHS_TEST2", "")
	defer os.Unsetenv("RTSP_PATHS_TEST2")

	// map values, "all" path
	os.Setenv("RTSP_PATHS_ALL_READUSER", "testuser")
	defer os.Unsetenv("RTSP_PATHS_ALL_READUSER")
	os.Setenv("RTSP_PATHS_ALL_READPASS", "testpass")
	defer os.Unsetenv("RTSP_PATHS_ALL_READPASS")

	// map values, generic path
	os.Setenv("RTSP_PATHS_CAM1_SOURCE", "rtsp://testing")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCE")
	os.Setenv("RTSP_PATHS_CAM1_SOURCEPROTOCOL", "tcp")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCEPROTOCOL")
	os.Setenv("RTSP_PATHS_CAM1_SOURCEONDEMAND", "yes")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCEONDEMAND")

	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	require.Equal(t, "test=cmd", p.conf.RunOnConnect)

	require.Equal(t, 8555, p.conf.RtspPort)

	require.Equal(t, true, p.conf.Metrics)

	require.Equal(t, 22*time.Second, p.conf.ReadTimeout)

	require.Equal(t, []string{"stdout", "file"}, p.conf.LogDestinations)

	pa, ok := p.conf.Paths["test2"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Source:                     "record",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)

	pa, ok = p.conf.Paths["~^.*$"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Regexp:                     regexp.MustCompile("^.*$"),
		Source:                     "record",
		SourceProtocol:             "automatic",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		ReadUser:                   "testuser",
		ReadPass:                   "testpass",
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)

	pa, ok = p.conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Source:         "rtsp://testing",
		SourceProtocol: "tcp",
		SourceProtocolParsed: func() *gortsplib.StreamProtocol {
			v := gortsplib.StreamProtocolTCP
			return &v
		}(),
		SourceOnDemand:             true,
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)
}

func TestEnvironmentNoFile(t *testing.T) {
	os.Setenv("RTSP_PATHS_CAM1_SOURCE", "rtsp://testing")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCE")

	p, ok := testProgram("{}")
	require.Equal(t, true, ok)
	defer p.close()

	pa, ok := p.conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, &conf.PathConf{
		Source:                     "rtsp://testing",
		SourceProtocol:             "automatic",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)
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

func TestPublishRead(t *testing.T) {
	for _, ca := range []struct {
		encrypted      bool
		publisherSoft  string
		publisherProto string
		readerSoft     string
		readerProto    string
	}{
		{false, "ffmpeg", "udp", "ffmpeg", "udp"},
		{false, "ffmpeg", "udp", "ffmpeg", "tcp"},
		{false, "ffmpeg", "udp", "gstreamer", "udp"},
		{false, "ffmpeg", "udp", "gstreamer", "tcp"},
		{false, "ffmpeg", "udp", "vlc", "udp"},
		{false, "ffmpeg", "udp", "vlc", "tcp"},

		{false, "ffmpeg", "tcp", "ffmpeg", "udp"},
		{false, "gstreamer", "udp", "ffmpeg", "udp"},
		{false, "gstreamer", "tcp", "ffmpeg", "udp"},

		{true, "ffmpeg", "tcp", "ffmpeg", "tcp"},
		{true, "ffmpeg", "tcp", "gstreamer", "tcp"},
		{true, "gstreamer", "tcp", "ffmpeg", "tcp"},
	} {
		encryptedStr := func() string {
			if ca.encrypted {
				return "encrypted"
			}
			return "plain"
		}()

		t.Run(encryptedStr+"_"+ca.publisherSoft+"_"+ca.publisherProto+"_"+
			ca.readerSoft+"_"+ca.readerProto, func(t *testing.T) {
			var proto string
			var port string
			if !ca.encrypted {
				proto = "rtsp"
				port = "8554"

				p, ok := testProgram("readTimeout: 20s")
				require.Equal(t, true, ok)
				defer p.close()

			} else {
				proto = "rtsps"
				port = "8555"

				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				p, ok := testProgram("readTimeout: 20s\n" +
					"protocols: [tcp]\n" +
					"encryption: yes\n" +
					"serverCert: " + serverCertFpath + "\n" +
					"serverKey: " + serverKeyFpath + "\n")
				require.Equal(t, true, ok)
				defer p.close()
			}

			time.Sleep(1 * time.Second)

			switch ca.publisherSoft {
			case "ffmpeg":
				cnt1, err := newContainer("ffmpeg", "source", []string{
					"-re",
					"-stream_loop", "-1",
					"-i", "emptyvideo.ts",
					"-c", "copy",
					"-f", "rtsp",
					"-rtsp_transport", ca.publisherProto,
					proto + "://" + ownDockerIP + ":" + port + "/teststream",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)

			case "gstreamer":
				cnt1, err := newContainer("gstreamer", "source", []string{
					"filesrc location=emptyvideo.ts ! tsdemux ! video/x-h264 ! rtspclientsink " +
						"location=" + proto + "://" + ownDockerIP + ":" + port + "/teststream " +
						"protocols=" + ca.publisherProto + " tls-validation-flags=0 latency=0 timeout=0 rtx-time=0",
				})
				require.NoError(t, err)
				defer cnt1.close()

				time.Sleep(1 * time.Second)
			}

			time.Sleep(1 * time.Second)

			switch ca.readerSoft {
			case "ffmpeg":
				cnt2, err := newContainer("ffmpeg", "dest", []string{
					"-rtsp_transport", ca.readerProto,
					"-i", proto + "://" + ownDockerIP + ":" + port + "/teststream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())

			case "gstreamer":
				cnt2, err := newContainer("gstreamer", "read", []string{
					"rtspsrc location=" + proto + "://" + ownDockerIP + ":" + port + "/teststream protocols=tcp tls-validation-flags=0 latency=0 " +
						"! application/x-rtp,media=video ! decodebin ! exitafterframe ! fakesink",
				})
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())

			case "vlc":
				args := []string{}
				if ca.readerProto == "tcp" {
					args = append(args, "--rtsp-tcp")
				}
				args = append(args, proto+"://"+ownDockerIP+":"+port+"/teststream")
				cnt2, err := newContainer("vlc", "dest", args)
				require.NoError(t, err)
				defer cnt2.close()
				require.Equal(t, 0, cnt2.wait())
			}
		})
	}
}

func TestTCPOnly(t *testing.T) {
	p, ok := testProgram("protocols: [tcp]\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "tcp",
		"rtsp://" + ownDockerIP + ":8554/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "tcp",
		"-i", "rtsp://" + ownDockerIP + ":8554/teststream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestPathWithSlash(t *testing.T) {
	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/test/stream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestPathWithQuery(t *testing.T) {
	p, ok := testProgram("")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test?param1=val&param2=val",
	})
	require.NoError(t, err)
	defer cnt1.close()

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/test?param3=otherval",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestAuth(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		p, ok := testProgram("paths:\n" +
			"  all:\n" +
			"    publishUser: testuser\n" +
			"    publishPass: test!$()*+.;<=>[]^_-{}\n" +
			"    publishIps: [172.17.0.0/16]\n")
		require.Equal(t, true, ok)
		defer p.close()

		time.Sleep(1 * time.Second)

		cnt1, err := newContainer("ffmpeg", "source", []string{
			"-re",
			"-stream_loop", "-1",
			"-i", "emptyvideo.ts",
			"-c", "copy",
			"-f", "rtsp",
			"-rtsp_transport", "udp",
			"rtsp://testuser:test!$()*+.;<=>[]^_-{}@" + ownDockerIP + ":8554/test/stream",
		})
		require.NoError(t, err)
		defer cnt1.close()

		time.Sleep(1 * time.Second)

		cnt2, err := newContainer("ffmpeg", "dest", []string{
			"-rtsp_transport", "udp",
			"-i", "rtsp://" + ownDockerIP + ":8554/test/stream",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt2.close()

		require.Equal(t, 0, cnt2.wait())
	})

	for _, soft := range []string{
		"ffmpeg",
		"vlc",
	} {
		t.Run("read_"+soft, func(t *testing.T) {
			p, ok := testProgram("paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: test!$()*+.;<=>[]^_-{}\n" +
				"    readIps: [172.17.0.0/16]\n")
			require.Equal(t, true, ok)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/test/stream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			if soft == "ffmpeg" {
				cnt2, err := newContainer("ffmpeg", "dest", []string{
					"-rtsp_transport", "udp",
					"-i", "rtsp://testuser:test!$()*+.;<=>[]^_-{}@" + ownDockerIP + ":8554/test/stream",
					"-vframes", "1",
					"-f", "image2",
					"-y", "/dev/null",
				})
				require.NoError(t, err)
				defer cnt2.close()

				require.Equal(t, 0, cnt2.wait())

			} else {
				cnt2, err := newContainer("vlc", "dest", []string{
					"rtsp://testuser:test!$()*+.;<=>[]^_-{}@" + ownDockerIP + ":8554/test/stream",
				})
				require.NoError(t, err)
				defer cnt2.close()

				require.Equal(t, 0, cnt2.wait())
			}
		})
	}
}

func TestAuthHashed(t *testing.T) {
	p, ok := testProgram("paths:\n" +
		"  all:\n" +
		"    readUser: sha256:rl3rgi4NcZkpAEcacZnQ2VuOfJ0FxAqCRaKB/SwdZoQ=\n" +
		"    readPass: sha256:E9JJ8stBJ7QM+nV4ZoUCeHk/gU3tPFh/5YieiJp6n2w=\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://testuser:testpass@" + ownDockerIP + ":8554/test/stream",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestAuthFail(t *testing.T) {
	for _, ca := range []struct {
		name string
		user string
		pass string
	}{
		{
			"publish_wronguser",
			"test1user",
			"testpass",
		},
		{
			"publish_wrongpass",
			"testuser",
			"test1pass",
		},
		{
			"publish_wrongboth",
			"test1user",
			"test1pass",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p, ok := testProgram("paths:\n" +
				"  all:\n" +
				"    publishUser: testuser\n" +
				"    publishPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ca.user + ":" + ca.pass + "@" + ownDockerIP + ":8554/test/stream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8554/test/stream",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()

			require.Equal(t, 1, cnt2.wait())
		})
	}

	for _, ca := range []struct {
		name string
		user string
		pass string
	}{
		{
			"read_wronguser",
			"test1user",
			"testpass",
		},
		{
			"read_wrongpass",
			"testuser",
			"test1pass",
		},
		{
			"read_wrongboth",
			"test1user",
			"test1pass",
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			p, ok := testProgram("paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: testpass\n")
			require.Equal(t, true, ok)
			defer p.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/test/stream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ca.user + ":" + ca.pass + "@" + ownDockerIP + ":8554/test/stream",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()

			require.Equal(t, 1, cnt2.wait())
		})
	}
}

func TestAuthIpFail(t *testing.T) {
	p, ok := testProgram("paths:\n" +
		"  all:\n" +
		"    publishIps: [127.0.0.1/32]\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/test/stream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	require.NotEqual(t, 0, cnt1.wait())
}

func TestSourceRtsp(t *testing.T) {
	for _, proto := range []string{
		"udp",
		"tcp",
	} {
		t.Run(proto, func(t *testing.T) {
			p1, ok := testProgram("paths:\n" +
				"  all:\n" +
				"    readUser: testuser\n" +
				"    readPass: testpass\n")
			require.Equal(t, true, ok)
			defer p1.close()

			time.Sleep(1 * time.Second)

			cnt1, err := newContainer("ffmpeg", "source", []string{
				"-re",
				"-stream_loop", "-1",
				"-i", "emptyvideo.ts",
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "udp",
				"rtsp://" + ownDockerIP + ":8554/teststream",
			})
			require.NoError(t, err)
			defer cnt1.close()

			time.Sleep(1 * time.Second)

			p2, ok := testProgram("rtspPort: 8555\n" +
				"rtpPort: 8100\n" +
				"rtcpPort: 8101\n" +
				"\n" +
				"paths:\n" +
				"  proxied:\n" +
				"    source: rtsp://testuser:testpass@localhost:8554/teststream\n" +
				"    sourceProtocol: " + proto + "\n" +
				"    sourceOnDemand: yes\n")
			require.Equal(t, true, ok)
			defer p2.close()

			time.Sleep(1 * time.Second)

			cnt2, err := newContainer("ffmpeg", "dest", []string{
				"-rtsp_transport", "udp",
				"-i", "rtsp://" + ownDockerIP + ":8555/proxied",
				"-vframes", "1",
				"-f", "image2",
				"-y", "/dev/null",
			})
			require.NoError(t, err)
			defer cnt2.close()

			require.Equal(t, 0, cnt2.wait())
		})
	}
}

func TestSourceRtsps(t *testing.T) {
	serverCertFpath, err := writeTempFile(serverCert)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := writeTempFile(serverKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	p, ok := testProgram("readTimeout: 20s\n" +
		"protocols: [tcp]\n" +
		"encryption: yes\n" +
		"serverCert: " + serverCertFpath + "\n" +
		"serverKey: " + serverKeyFpath + "\n" +
		"paths:\n" +
		"  all:\n" +
		"    readUser: testuser\n" +
		"    readPass: testpass\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"rtsps://" + ownDockerIP + ":8555/teststream",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	p2, ok := testProgram("rtspPort: 8556\n" +
		"rtpPort: 8100\n" +
		"rtcpPort: 8101\n" +
		"\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsps://testuser:testpass@localhost:8555/teststream\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p2.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8556/proxied",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestSourceRtmp(t *testing.T) {
	cnt1, err := newContainer("nginx-rtmp", "rtmpserver", []string{})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "flv",
		"rtmp://" + cnt1.ip() + "/stream/test",
	})
	require.NoError(t, err)
	defer cnt2.close()

	time.Sleep(1 * time.Second)

	p, ok := testProgram("paths:\n" +
		"  proxied:\n" +
		"    source: rtmp://" + cnt1.ip() + "/stream/test\n" +
		"    sourceOnDemand: yes\n")
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	cnt3, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/proxied",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt3.close()

	require.Equal(t, 0, cnt3.wait())
}

func TestRedirect(t *testing.T) {
	p1, ok := testProgram("paths:\n" +
		"  path1:\n" +
		"    source: redirect\n" +
		"    sourceRedirect: rtsp://" + ownDockerIP + ":8554/path2\n" +
		"  path2:\n")
	require.Equal(t, true, ok)
	defer p1.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/path2",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/path1",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestFallback(t *testing.T) {
	p1, ok := testProgram("paths:\n" +
		"  path1:\n" +
		"    fallback: rtsp://" + ownDockerIP + ":8554/path2\n" +
		"  path2:\n")
	require.Equal(t, true, ok)
	defer p1.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "source", []string{
		"-re",
		"-stream_loop", "-1",
		"-i", "emptyvideo.ts",
		"-c", "copy",
		"-f", "rtsp",
		"-rtsp_transport", "udp",
		"rtsp://" + ownDockerIP + ":8554/path2",
	})
	require.NoError(t, err)
	defer cnt1.close()

	time.Sleep(1 * time.Second)

	cnt2, err := newContainer("ffmpeg", "dest", []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://" + ownDockerIP + ":8554/path1",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt2.close()

	require.Equal(t, 0, cnt2.wait())
}

func TestRunOnDemand(t *testing.T) {
	p1, ok := testProgram("paths:\n" +
		"  all:\n" +
		"    runOnDemand: ffmpeg -hide_banner -loglevel error -re -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH\n")
	require.Equal(t, true, ok)
	defer p1.close()

	time.Sleep(1 * time.Second)

	cnt1, err := newContainer("ffmpeg", "dest", []string{
		"-i", "rtsp://" + ownDockerIP + ":8554/ondemand",
		"-vframes", "1",
		"-f", "image2",
		"-y", "/dev/null",
	})
	require.NoError(t, err)
	defer cnt1.close()

	require.Equal(t, 0, cnt1.wait())
}

func TestHotReloading(t *testing.T) {
	confPath := filepath.Join(os.TempDir(), "rtsp-conf")

	err := ioutil.WriteFile(confPath, []byte("paths:\n"+
		"  test1:\n"+
		"    runOnDemand: ffmpeg -hide_banner -loglevel error -re -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH\n"+
		"  test3:\n"+
		"    runOnInit: echo aaa\n"+
		"  test4:\n"+
		"    runOnInit: echo bbb\n"),
		0644)
	require.NoError(t, err)
	defer os.Remove(confPath)

	p, ok := newProgram([]string{confPath})
	require.Equal(t, true, ok)
	defer p.close()

	time.Sleep(1 * time.Second)

	func() {
		cnt1, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtsp://" + ownDockerIP + ":8554/test1",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt1.close()

		require.Equal(t, 0, cnt1.wait())
	}()

	err = ioutil.WriteFile(confPath, []byte("paths:\n"+
		"  test2:\n"+
		"    runOnDemand: ffmpeg -hide_banner -loglevel error -re -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH\n"+
		"  test3:\n"+
		"  test4:\n"+
		"    runOnInit: echo bbb\n"),
		0644)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	func() {
		cnt1, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtsp://" + ownDockerIP + ":8554/test1",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt1.close()

		require.Equal(t, 1, cnt1.wait())
	}()

	func() {
		cnt1, err := newContainer("ffmpeg", "dest", []string{
			"-i", "rtsp://" + ownDockerIP + ":8554/test2",
			"-vframes", "1",
			"-f", "image2",
			"-y", "/dev/null",
		})
		require.NoError(t, err)
		defer cnt1.close()

		require.Equal(t, 0, cnt1.wait())
	}()
}
