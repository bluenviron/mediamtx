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

	"github.com/bluenviron/mediamtx/internal/conf"
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

// CertLoader is a certificate loader.
// It loads a default certificate and optional additional certificates,
// watches for changes to their files and picks a certificate
// by the server name requested by the client (TLS SNI).
// When CertPath and KeyPath are empty, the first additional certificate
// is the default one.
type CertLoader struct {
	CertPath        string
	KeyPath         string
	AllowAuto       bool
	AdditionalCerts []conf.TLSCert
	Parent          logger.Writer

	loaders []*keyPairLoader
}

// Initialize initializes a CertLoader.
func (cl *CertLoader) Initialize() error {
	var loaders []*keyPairLoader

	if cl.CertPath != "" || cl.KeyPath != "" || len(cl.AdditionalCerts) == 0 {
		loaders = append(loaders, &keyPairLoader{
			certPath:  cl.CertPath,
			keyPath:   cl.KeyPath,
			allowAuto: cl.AllowAuto,
			parent:    cl.Parent,
		})
	}

	for _, c := range cl.AdditionalCerts {
		loaders = append(loaders, &keyPairLoader{
			certPath: c.Cert,
			keyPath:  c.Key,
			parent:   cl.Parent,
		})
	}

	for _, l := range loaders {
		err := l.initialize()
		if err != nil {
			cl.Close()
			return err
		}
		cl.loaders = append(cl.loaders, l)
	}

	return nil
}

// Close closes a CertLoader and releases any underlying resources.
func (cl *CertLoader) Close() {
	for _, l := range cl.loaders {
		l.close()
	}
}

// GetCertificate returns the certificate for use in a tls.Config.
// It returns the first certificate that is valid for the server name
// requested by the client, or the default certificate if none is.
func (cl *CertLoader) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello != nil && len(cl.loaders) > 1 {
		for _, l := range cl.loaders {
			cert := l.get()
			if cert != nil && hello.SupportsCertificate(cert) == nil {
				return cert, nil
			}
		}
	}

	return cl.loaders[0].get(), nil
}

// keyPairLoader loads a single key pair and watches for changes to its files.
type keyPairLoader struct {
	certPath  string
	keyPath   string
	allowAuto bool
	parent    logger.Writer

	certWatcher, keyWatcher *confwatcher.ConfWatcher
	cert                    *tls.Certificate
	certMu                  sync.RWMutex

	terminate chan struct{}
	done      chan struct{}
}

func (cl *keyPairLoader) initialize() error {
	cl.terminate = make(chan struct{})
	cl.done = make(chan struct{})

	if cl.allowAuto && cl.keyPath == autoKeyName && cl.certPath == autoCertName {
		watch, err := cl.initializeAuto()
		if err != nil {
			return err
		}
		if !watch {
			close(cl.done)
			return nil
		}
	} else {
		cert, err := tls.LoadX509KeyPair(cl.certPath, cl.keyPath)
		if err != nil {
			return err
		}
		cl.certMu.Lock()
		cl.cert = &cert
		cl.certMu.Unlock()
	}

	cl.certWatcher = &confwatcher.ConfWatcher{FilePath: cl.certPath}
	if err := cl.certWatcher.Initialize(); err != nil {
		return err
	}

	cl.keyWatcher = &confwatcher.ConfWatcher{FilePath: cl.keyPath}
	if err := cl.keyWatcher.Initialize(); err != nil {
		cl.certWatcher.Close() //nolint:errcheck
		return err
	}

	go cl.watch()

	return nil
}

func (cl *keyPairLoader) initializeAuto() (bool, error) {
	_, keyErr := os.Stat(cl.keyPath)
	_, certErr := os.Stat(cl.certPath)

	if keyErr == nil && certErr == nil {
		cert, err := tls.LoadX509KeyPair(cl.certPath, cl.keyPath)
		if err != nil {
			return false, err
		}
		cl.certMu.Lock()
		cl.cert = &cert
		cl.certMu.Unlock()
		return true, nil
	}

	cl.parent.Log(logger.Warn, "certificate %s not found, generating it from scratch", cl.keyPath)

	certPEM, keyPEM, err := generateTLSCert()
	if err != nil {
		return false, err
	}

	keyWriteErr := os.WriteFile(cl.keyPath, keyPEM, 0o600)
	if keyWriteErr != nil {
		cl.parent.Log(logger.Warn, "failed to save TLS key to %s: %v", cl.keyPath, keyWriteErr)
	}
	certWriteErr := os.WriteFile(cl.certPath, certPEM, 0o600)
	if certWriteErr != nil {
		cl.parent.Log(logger.Warn, "failed to save TLS cert to %s: %v", cl.certPath, certWriteErr)
	}

	cert, _ := tls.X509KeyPair(certPEM, keyPEM)
	cl.certMu.Lock()
	cl.cert = &cert
	cl.certMu.Unlock()

	return keyWriteErr == nil && certWriteErr == nil, nil
}

func (cl *keyPairLoader) close() {
	close(cl.terminate)
	<-cl.done

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

func (cl *keyPairLoader) get() *tls.Certificate {
	cl.certMu.RLock()
	defer cl.certMu.RUnlock()
	return cl.cert
}

func (cl *keyPairLoader) watch() {
	defer close(cl.done)

	for {
		select {
		case <-cl.certWatcher.Watch():
			cert, err := tls.LoadX509KeyPair(cl.certPath, cl.keyPath)
			if err != nil {
				cl.parent.Log(logger.Error, "certloader failed to load after change to %s: %s", cl.certPath, err.Error())
				continue
			}

			cl.certMu.Lock()
			cl.cert = &cert
			cl.certMu.Unlock()

			cl.parent.Log(logger.Info, "certificate reloaded after change to %s", cl.certPath)

		case <-cl.keyWatcher.Watch():
			cert, err := tls.LoadX509KeyPair(cl.certPath, cl.keyPath)
			if err != nil {
				cl.parent.Log(logger.Error, "certloader failed to load after change to %s: %s", cl.keyPath, err.Error())
				continue
			}

			cl.certMu.Lock()
			cl.cert = &cert
			cl.certMu.Unlock()

			cl.parent.Log(logger.Info, "certificate reloaded after change to %s", cl.keyPath)

		case <-cl.terminate:
			return
		}
	}
}
