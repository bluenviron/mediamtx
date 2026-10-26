package certloader_test

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestCertReload(t *testing.T) {
	testData, err := tls.X509KeyPair(test.TLSCertPub, test.TLSCertKey)
	require.NoError(t, err)

	serverCertPath := test.CreateTempFile(t, test.TLSCertPub)
	serverKeyPath := test.CreateTempFile(t, test.TLSCertKey)

	loader := &certloader.CertLoader{
		CertPath: serverCertPath,
		KeyPath:  serverKeyPath,
		Parent:   test.NilLogger,
	}
	err = loader.Initialize()
	require.NoError(t, err)
	defer loader.Close()

	cert, err := loader.GetCertificate(nil)
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

	cert, err = loader.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, &testData, cert)
}

func TestCloseAutoCert(t *testing.T) {
	for _, ca := range []struct {
		name  string
		saved bool
	}{
		{"saved", true},
		{"not saved", false},
	} {
		t.Run(ca.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if !ca.saved {
				// a directory with the same name prevents the key from being saved
				err := os.Mkdir("auto.key", 0o755)
				require.NoError(t, err)
			}

			loader := &certloader.CertLoader{
				CertPath:  "auto.crt",
				KeyPath:   "auto.key",
				AllowAuto: true,
				Parent:    test.NilLogger,
			}
			err := loader.Initialize()
			require.NoError(t, err)

			done := make(chan struct{})
			go func() {
				loader.Close()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Close() did not return")
			}
		})
	}
}
