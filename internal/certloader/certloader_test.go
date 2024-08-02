package certloader

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestCertReload(t *testing.T) {
	testData, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	serverCertPath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertPath)

	serverKeyPath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyPath)

	loader, err := New(serverCertPath, serverKeyPath, test.NilLogger)
	require.NoError(t, err)
	defer loader.Close()

	getCert := loader.GetCertificate()
	require.NotNil(t, getCert)

	cert, err := getCert(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, &testData, cert)

	testData, err = tls.X509KeyPair(test.TLSCertPubAlt, test.TLSCertKeyAlt)
	require.NoError(t, err)

	err = os.WriteFile(serverCertPath, test.TLSCertPubAlt, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(serverKeyPath, test.TLSCertKeyAlt, 0o644)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	cert, err = getCert(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, &testData, cert)
}
