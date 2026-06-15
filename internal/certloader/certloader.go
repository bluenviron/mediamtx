// Package certloader contains a certicate loader.
package certloader

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/confwatcher"
	"github.com/bluenviron/mediamtx/internal/logger"
)

const (
	autoKeyName  = "auto.key"
	autoCertName = "auto.crt"
)

func generateTLSCert() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mediamtx"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// CertLoader is a certificate loader. It watches for changes to the certificate and key files.
type CertLoader struct {
	CertPath  string
	KeyPath   string
	AllowAuto bool
	Parent    logger.Writer

	certWatcher, keyWatcher *confwatcher.ConfWatcher
	cert                    *tls.Certificate
	certMu                  sync.RWMutex

	done chan struct{}
}

// Initialize initializes a CertLoader.
func (cl *CertLoader) Initialize() error {
	cl.done = make(chan struct{})

	if cl.AllowAuto && cl.KeyPath == autoKeyName && cl.CertPath == autoCertName {
		watch, err := cl.initializeAuto()
		if err != nil {
			return err
		}
		if !watch {
			return nil
		}
	} else {
		cert, err := tls.LoadX509KeyPair(cl.CertPath, cl.KeyPath)
		if err != nil {
			return err
		}
		cl.certMu.Lock()
		cl.cert = &cert
		cl.certMu.Unlock()
	}

	cl.certWatcher = &confwatcher.ConfWatcher{FilePath: cl.CertPath}
	if err := cl.certWatcher.Initialize(); err != nil {
		return err
	}

	cl.keyWatcher = &confwatcher.ConfWatcher{FilePath: cl.KeyPath}
	if err := cl.keyWatcher.Initialize(); err != nil {
		cl.certWatcher.Close() //nolint:errcheck
		return err
	}

	go cl.watch()

	return nil
}

func (cl *CertLoader) initializeAuto() (bool, error) {
	_, keyErr := os.Stat(cl.KeyPath)
	_, certErr := os.Stat(cl.CertPath)

	if keyErr == nil && certErr == nil {
		cert, err := tls.LoadX509KeyPair(cl.CertPath, cl.KeyPath)
		if err != nil {
			return false, err
		}
		cl.certMu.Lock()
		cl.cert = &cert
		cl.certMu.Unlock()
		return true, nil
	}

	cl.Parent.Log(logger.Warn, "certificate %s not found, generating it from scratch", cl.KeyPath)

	certPEM, keyPEM, err := generateTLSCert()
	if err != nil {
		return false, err
	}

	keyWriteErr := os.WriteFile(cl.KeyPath, keyPEM, 0o600)
	if keyWriteErr != nil {
		cl.Parent.Log(logger.Warn, "failed to save TLS key to %s: %v", cl.KeyPath, keyWriteErr)
	}
	certWriteErr := os.WriteFile(cl.CertPath, certPEM, 0o600)
	if certWriteErr != nil {
		cl.Parent.Log(logger.Warn, "failed to save TLS cert to %s: %v", cl.CertPath, certWriteErr)
	}

	cert, _ := tls.X509KeyPair(certPEM, keyPEM)
	cl.certMu.Lock()
	cl.cert = &cert
	cl.certMu.Unlock()

	return keyWriteErr == nil && certWriteErr == nil, nil
}

// Close closes a CertLoader and releases any underlying resources.
func (cl *CertLoader) Close() {
	close(cl.done)
	if cl.certWatcher != nil {
		cl.certWatcher.Close() //nolint:errcheck
	}
	if cl.keyWatcher != nil {
		cl.keyWatcher.Close() //nolint:errcheck
	}
	cl.certMu.Lock()
	defer cl.certMu.Unlock()
	cl.cert = nil
}

// GetCertificate returns a function that returns the certificate for use in a tls.Config.
func (cl *CertLoader) GetCertificate() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		cl.certMu.RLock()
		defer cl.certMu.RUnlock()
		return cl.cert, nil
	}
}

func (cl *CertLoader) watch() {
	for {
		select {
		case <-cl.certWatcher.Watch():
			cert, err := tls.LoadX509KeyPair(cl.CertPath, cl.KeyPath)
			if err != nil {
				cl.Parent.Log(logger.Error, "certloader failed to load after change to %s: %s", cl.CertPath, err.Error())
				continue
			}

			cl.certMu.Lock()
			cl.cert = &cert
			cl.certMu.Unlock()

			cl.Parent.Log(logger.Info, "certificate reloaded after change to %s", cl.CertPath)

		case <-cl.keyWatcher.Watch():
			cert, err := tls.LoadX509KeyPair(cl.CertPath, cl.KeyPath)
			if err != nil {
				cl.Parent.Log(logger.Error, "certloader failed to load after change to %s: %s", cl.KeyPath, err.Error())
				continue
			}

			cl.certMu.Lock()
			cl.cert = &cert
			cl.certMu.Unlock()

			cl.Parent.Log(logger.Info, "certificate reloaded after change to %s", cl.KeyPath)

		case <-cl.done:
			return
		}
	}
}
