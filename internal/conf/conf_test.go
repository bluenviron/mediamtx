package conf

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

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

func TestWithFileAndEnv(t *testing.T) {
	os.Setenv("RTSP_PATHS_CAM1_SOURCE", "rtsp://testing")
	defer os.Unsetenv("RTSP_PATHS_CAM1_SOURCE")

	tmpf, err := writeTempFile([]byte("{}"))
	require.NoError(t, err)
	defer os.Remove(tmpf)

	conf, hasFile, err := Load(tmpf)
	require.NoError(t, err)
	require.Equal(t, true, hasFile)

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

func TestWithEnvOnly(t *testing.T) {
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
