// Package certloader contains a certicate loader.
package certloader

import (
	"crypto/tls"
	"sync"

	"github.com/bluenviron/mediamtx/internal/confwatcher"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// CertLoader is a certificate loader. It watches for changes to the certificate and key files.
type CertLoader struct {
	CertPath string
	KeyPath  string
	Parent   logger.Writer

	certWatcher, keyWatcher *confwatcher.ConfWatcher
	cert                    *tls.Certificate
	certMu                  sync.RWMutex

	done chan struct{}
}

// Initialize initializes a CertLoader.
func (cl *CertLoader) Initialize() error {
	cl.done = make(chan struct{})

	cl.certWatcher = &confwatcher.ConfWatcher{FilePath: cl.CertPath}
	err := cl.certWatcher.Initialize()
	if err != nil {
		return err
	}

	cl.keyWatcher = &confwatcher.ConfWatcher{FilePath: cl.KeyPath}
	err = cl.keyWatcher.Initialize()
	if err != nil {
		cl.certWatcher.Close() //nolint:errcheck
		return err
	}

	cert, err := tls.LoadX509KeyPair(cl.CertPath, cl.KeyPath)
	if err != nil {
		return err
	}

	cl.certMu.Lock()
	cl.cert = &cert
	cl.certMu.Unlock()

	go cl.watch()

	return nil
}

// Close closes a CertLoader and releases any underlying resources.
func (cl *CertLoader) Close() {
	close(cl.done)
	cl.certWatcher.Close() //nolint:errcheck
	cl.keyWatcher.Close()  //nolint:errcheck
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
