package certloader_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/conf"
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

func generateCert(t *testing.T, dnsName string) (string, string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	certPath := test.CreateTempFile(t, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	keyPath := test.CreateTempFile(t, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))

	return certPath, keyPath
}

func handshake(t *testing.T, loader *certloader.CertLoader, serverName string) string {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	server := tls.Server(serverConn, &tls.Config{GetCertificate: loader.GetCertificate})
	go server.Handshake() //nolint:errcheck

	client := tls.Client(clientConn, &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true,
	})
	err := client.Handshake()
	require.NoError(t, err)

	return client.ConnectionState().PeerCertificates[0].Subject.CommonName
}

func TestCertSelectionBySNI(t *testing.T) {
	defaultCert, defaultKey := generateCert(t, "default.example.com")
	extra1Cert, extra1Key := generateCert(t, "extra1.example.com")
	extra2Cert, extra2Key := generateCert(t, "extra2.example.com")

	loader := &certloader.CertLoader{
		CertPath: defaultCert,
		KeyPath:  defaultKey,
		AdditionalCerts: []conf.TLSCert{
			{Key: extra1Key, Cert: extra1Cert},
			{Key: extra2Key, Cert: extra2Cert},
		},
		Parent: test.NilLogger,
	}
	err := loader.Initialize()
	require.NoError(t, err)
	defer loader.Close()

	for _, ca := range []struct {
		serverName string
		expected   string
	}{
		{"default.example.com", "default.example.com"},
		{"extra1.example.com", "extra1.example.com"},
		{"extra2.example.com", "extra2.example.com"},
		{"unknown.example.com", "default.example.com"},
		{"", "default.example.com"},
	} {
		t.Run("sni "+ca.serverName, func(t *testing.T) {
			require.Equal(t, ca.expected, handshake(t, loader, ca.serverName))
		})
	}
}

func TestAdditionalCertReload(t *testing.T) {
	defaultCert, defaultKey := generateCert(t, "default.example.com")
	extraCert, extraKey := generateCert(t, "extra.example.com")

	loader := &certloader.CertLoader{
		CertPath:        defaultCert,
		KeyPath:         defaultKey,
		AdditionalCerts: []conf.TLSCert{{Key: extraKey, Cert: extraCert}},
		Parent:          test.NilLogger,
	}
	err := loader.Initialize()
	require.NoError(t, err)
	defer loader.Close()

	require.Equal(t, "extra.example.com", handshake(t, loader, "extra.example.com"))
	require.Equal(t, "default.example.com", handshake(t, loader, "renamed.example.com"))

	newCert, newKey := generateCert(t, "renamed.example.com")

	buf, err := os.ReadFile(newCert)
	require.NoError(t, err)
	err = os.WriteFile(extraCert, buf, 0o644)
	require.NoError(t, err)

	buf, err = os.ReadFile(newKey)
	require.NoError(t, err)
	err = os.WriteFile(extraKey, buf, 0o644)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	require.Equal(t, "renamed.example.com", handshake(t, loader, "renamed.example.com"))
	require.Equal(t, "default.example.com", handshake(t, loader, "extra.example.com"))
}

func TestAdditionalCertInvalid(t *testing.T) {
	defaultCert, defaultKey := generateCert(t, "default.example.com")

	loader := &certloader.CertLoader{
		CertPath:        defaultCert,
		KeyPath:         defaultKey,
		AdditionalCerts: []conf.TLSCert{{Key: "/nonexistent.key", Cert: "/nonexistent.crt"}},
		Parent:          test.NilLogger,
	}
	err := loader.Initialize()
	require.Error(t, err)
}

func TestDefaultFromAdditionalCerts(t *testing.T) {
	cert1, key1 := generateCert(t, "one.example.com")
	cert2, key2 := generateCert(t, "two.example.com")

	loader := &certloader.CertLoader{
		AdditionalCerts: []conf.TLSCert{
			{Key: key1, Cert: cert1},
			{Key: key2, Cert: cert2},
		},
		Parent: test.NilLogger,
	}
	err := loader.Initialize()
	require.NoError(t, err)
	defer loader.Close()

	require.Equal(t, "one.example.com", handshake(t, loader, "one.example.com"))
	require.Equal(t, "two.example.com", handshake(t, loader, "two.example.com"))
	require.Equal(t, "one.example.com", handshake(t, loader, "unknown.example.com"))
	require.Equal(t, "one.example.com", handshake(t, loader, ""))
}
