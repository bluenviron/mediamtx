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
	log                     logger.Writer
	certWatcher, keyWatcher *confwatcher.ConfWatcher
	certPath, keyPath       string
	done                    chan struct{}

	cert   *tls.Certificate
	certMu sync.RWMutex
}

// New allocates a CertLoader.
func New(certPath, keyPath string, log logger.Writer) (*CertLoader, error) {
	cl := &CertLoader{
		log:      log,
		certPath: certPath,
		keyPath:  keyPath,
		done:     make(chan struct{}),
	}

	var err error
	cl.certWatcher, err = confwatcher.New(certPath)
	if err != nil {
		return nil, err
	}

	cl.keyWatcher, err = confwatcher.New(keyPath)
	if err != nil {
		cl.certWatcher.Close() //nolint:errcheck
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	cl.certMu.Lock()
	cl.cert = &cert
	cl.certMu.Unlock()

	go cl.watch()

	return cl, nil
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
			cert, err := tls.LoadX509KeyPair(cl.certPath, cl.keyPath)
			if err != nil {
				cl.log.Log(logger.Error, "certloader failed to load after change to %s: %s", cl.certPath, err.Error())
				continue
			}

			cl.certMu.Lock()
			cl.cert = &cert
			cl.certMu.Unlock()

			cl.log.Log(logger.Info, "certificate reloaded after change to %s", cl.certPath)
		case <-cl.keyWatcher.Watch():
			cert, err := tls.LoadX509KeyPair(cl.certPath, cl.keyPath)
			if err != nil {
				cl.log.Log(logger.Error, "certloader failed to load after change to %s: %s", cl.keyPath, err.Error())
				continue
			}

			cl.certMu.Lock()
			cl.cert = &cert
			cl.certMu.Unlock()

			cl.log.Log(logger.Info, "certificate reloaded after change to %s", cl.keyPath)
		case <-cl.done:
			return
		}
	}
}
