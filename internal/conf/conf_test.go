package conf

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/aler9/gortsplib"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/secretbox"
)

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

	tmpf, err := writeTempFile([]byte("{}"))
	require.NoError(t, err)
	defer os.Remove(tmpf)

	conf, hasFile, err := Load(tmpf)
	require.NoError(t, err)
	require.Equal(t, true, hasFile)

	require.Equal(t, "test=cmd", conf.RunOnConnect)

	require.Equal(t, 8555, conf.RTSPPort)

	require.Equal(t, true, conf.Metrics)

	require.Equal(t, 22*time.Second, conf.ReadTimeout)

	require.Equal(t, []string{"stdout", "file"}, conf.LogDestinations)

	pa, ok := conf.Paths["test2"]
	require.Equal(t, true, ok)
	require.Equal(t, &PathConf{
		Source:                     "record",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)

	pa, ok = conf.Paths["~^.*$"]
	require.Equal(t, true, ok)
	require.Equal(t, &PathConf{
		Regexp:                     regexp.MustCompile("^.*$"),
		Source:                     "record",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		ReadUser:                   "testuser",
		ReadPass:                   "testpass",
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)

	pa, ok = conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, &PathConf{
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

	conf, hasFile, err := Load("rtsp-simple-server.yml")
	require.NoError(t, err)
	require.Equal(t, false, hasFile)

	pa, ok := conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, &PathConf{
		Source:                     "rtsp://testing",
		SourceProtocol:             "automatic",
		SourceOnDemandStartTimeout: 10 * time.Second,
		SourceOnDemandCloseAfter:   10 * time.Second,
		RunOnDemandStartTimeout:    10 * time.Second,
		RunOnDemandCloseAfter:      10 * time.Second,
	}, pa)
}

func TestEncryption(t *testing.T) {
	key := "testing123testin"
	plaintext := `
paths:
  path1:
  path2:
`

	encryptedConf := func() string {
		var secretKey [32]byte
		copy(secretKey[:], key)

		var nonce [24]byte
		if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
			panic(err)
		}

		encrypted := secretbox.Seal(nonce[:], []byte(plaintext), &nonce, &secretKey)
		return base64.StdEncoding.EncodeToString(encrypted)
	}()

	os.Setenv("RTSP_CONFKEY", key)
	defer os.Unsetenv("RTSP_CONFKEY")

	tmpf, err := writeTempFile([]byte(encryptedConf))
	require.NoError(t, err)
	defer os.Remove(tmpf)

	conf, hasFile, err := Load(tmpf)
	require.NoError(t, err)
	require.Equal(t, true, hasFile)

	_, ok := conf.Paths["path1"]
	require.Equal(t, true, ok)

	_, ok = conf.Paths["path2"]
	require.Equal(t, true, ok)
}
