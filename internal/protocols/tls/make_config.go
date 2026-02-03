// Package tls contains TLS utilities.
package tls

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// MakeConfig returns a tls.Config with:
// - server name indicator (SNI) support
// - fingerprint support
func MakeConfig(serverName string, fingerprint string) *tls.Config {
	conf := &tls.Config{
		ServerName: serverName,
	}

	if fingerprint != "" {
		fingerprintLower := strings.ToLower(fingerprint)
		conf.InsecureSkipVerify = true
		conf.VerifyConnection = func(cs tls.ConnectionState) error {
			h := sha256.New()
			h.Write(cs.PeerCertificates[0].Raw)
			hstr := hex.EncodeToString(h.Sum(nil))

			if hstr != fingerprintLower {
				return fmt.Errorf("source fingerprint does not match: expected %s, got %s",
					fingerprintLower, hstr)
			}

			return nil
		}
	}

	return conf
}

// MakeConfigWithCA returns a tls.Config with:
// - server name indicator (SNI) support
// - custom CA certificate pool from file
func MakeConfigWithCA(serverName string, caFile string) (*tls.Config, error) {
	conf := &tls.Config{
		ServerName: serverName,
	}

	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		conf.RootCAs = caCertPool
	}

	return conf, nil
}
