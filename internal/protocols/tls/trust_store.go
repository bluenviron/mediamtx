package tls

import (
	"crypto/x509"
	"fmt"
	"os"
)

// LoadTrustStore from a file in PEM format. The loaded certificates are merged with system cert pool if available.
func LoadTrustStore(trustStore string) (*x509.CertPool, error) {
	pemContent, err := os.ReadFile(trustStore)
	if err != nil {
		return nil, fmt.Errorf("unable to load trust store from [%s]: %w", trustStore, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemContent) {
		return nil, fmt.Errorf("no certificates loaded from provided trust store [%s]", trustStore)
	}
	return pool, nil
}
