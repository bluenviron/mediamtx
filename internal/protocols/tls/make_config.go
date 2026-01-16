// Package tls contains TLS utilities.
package tls

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"
)

// MakeConfig returns a tls.Config with:
// - server name indicator (SNI) support
// - fingerprint support
// - trust store support
func MakeConfig(serverName string, fingerprint string, trustStore *x509.CertPool) *tls.Config {
	conf := &tls.Config{
		ServerName: serverName,
		RootCAs:    trustStore,
	}

	if fingerprint != "" {
		fingerprintLower := strings.ToLower(fingerprint)
		conf.InsecureSkipVerify = true
		conf.VerifyConnection = func(cs tls.ConnectionState) error {
			return verifyFingerprint(cs.PeerCertificates, fingerprintLower)
		}
	}

	return conf
}

func verifyFingerprint(chain []*x509.Certificate, expectedFingerprint string) error {
	var certFingerprint string
	fingerprints := make([]string, 0, len(chain))
	hash := sha256.New()
	for _, cert := range chain {
		hash.Write(cert.Raw)
		certFingerprint = hex.EncodeToString(hash.Sum(nil))
		if certFingerprint == expectedFingerprint {
			return nil
		}
		fingerprints = append(fingerprints, certFingerprint)
		hash.Reset()
	}
	return fmt.Errorf("no certificate with matching fingerprint found, expected [%s] to be included in [%s]",
		expectedFingerprint,
		strings.Join(fingerprints, ", "),
	)
}
